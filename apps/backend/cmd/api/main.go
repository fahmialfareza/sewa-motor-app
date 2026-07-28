package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	exportadapter "github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/adapter/export"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/adapter/httpapi"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/adapter/postgres"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/adapter/redisinfra"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/adapter/security"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/config"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/port"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/usecase"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/migrations"
	"github.com/newrelic/go-agent/v3/newrelic"
)

func main() {
	if err := run(); err != nil {
		observability.Logger().WithError(err).Error("api stopped")
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	telemetry, err := observability.New(observability.Config{
		Enabled:            cfg.NewRelicEnabled,
		AppName:            cfg.NewRelicAppName,
		LicenseKey:         cfg.NewRelicLicenseKey,
		DistributedTracing: cfg.NewRelicDistributedTracing,
		LogForwarding:      cfg.NewRelicLogForwarding,
		LogLevel:           cfg.LogLevel,
	})
	if err != nil {
		return err
	}
	defer telemetry.Shutdown(5 * time.Second)
	logger := telemetry.Logger
	if err := telemetry.WaitForConnection(5 * time.Second); err != nil {
		logger.WithError(err).Warn("New Relic connection is not ready; telemetry will retry in the background")
	}
	connectCtx, connectCancel := context.WithTimeout(context.Background(), 15*time.Second)
	store, err := postgres.Open(connectCtx, cfg.DatabaseURL, postgres.WithLogger(logger))
	connectCancel()
	if err != nil {
		return err
	}
	defer store.Close()
	if cfg.AutoMigrate {
		migrationCtx, migrationCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		migrationTransaction := telemetry.App.StartTransaction("DatabaseMigration")
		migrationCtx = newrelic.NewContext(migrationCtx, migrationTransaction)
		err := migrations.Apply(migrationCtx, store.ORM)
		migrationCancel()
		migrationTransaction.End()
		if err != nil {
			return err
		}
	}

	var sessionIndex port.SessionIndex = redisinfra.Noop{}
	var limiter port.RateLimiter = redisinfra.Noop{}
	var redisPinger httpapi.Pinger
	var redisCache *redisinfra.Cache
	if cfg.RedisURL != "" {
		redisCache, err = redisinfra.New(cfg.RedisURL, cfg.SessionCacheTTL)
		if err != nil {
			return err
		}
		defer redisCache.Close()
		sessionIndex = redisCache
		limiter = redisCache
		redisPinger = redisCache
		pingCtx, pingCancel := context.WithTimeout(context.Background(), time.Second)
		if pingErr := redisCache.Ping(pingCtx); pingErr != nil {
			logger.WithError(pingErr).Warn("redis unavailable; continuing with PostgreSQL correctness")
		}
		pingCancel()
	}

	passwords := security.DefaultArgon2id()
	auth := usecase.Auth{
		Repo: store, Passwords: passwords, Tokens: security.OpaqueTokenManager{},
		Sessions: sessionIndex, Limiter: limiter,
		RateLimit: cfg.LoginRateLimit, RateWindow: cfg.LoginRateWindow,
	}
	transactions := usecase.Transactions{Repo: store, Clock: port.SystemClock{}}
	router := httpapi.New(httpapi.Dependencies{
		Repo: store, Auth: auth,
		Users:        usecase.Users{Repo: store, Passwords: passwords},
		Packages:     usecase.Packages{Repo: store},
		Transactions: transactions,
		Reporting:    usecase.Reporting{Repo: store, Exporter: exportadapter.Generator{}},
		Terminals:    usecase.Terminals{Repo: store},
		Sync:         usecase.Sync{Repo: store, Transactions: transactions},
		Redis:        redisPinger, Logger: logger, NewRelic: telemetry.App,
	})
	if err := router.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return err
	}
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	shutdownSignal, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	serverError := make(chan error, 1)
	go func() {
		logger.WithField("address", cfg.HTTPAddr).Info("api listening")
		serverError <- server.ListenAndServe()
	}()
	select {
	case err := <-serverError:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-shutdownSignal.Done():
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	return server.Shutdown(shutdownCtx)
}
