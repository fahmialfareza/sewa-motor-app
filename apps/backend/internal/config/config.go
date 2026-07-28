package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr                   string
	DatabaseURL                string
	RedisURL                   string
	AutoMigrate                bool
	LogLevel                   string
	ShutdownTimeout            time.Duration
	SessionCacheTTL            time.Duration
	LoginRateLimit             int
	LoginRateWindow            time.Duration
	TrustedProxies             []string
	NewRelicEnabled            bool
	NewRelicAppName            string
	NewRelicLicenseKey         string
	NewRelicDistributedTracing bool
	NewRelicLogForwarding      bool
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:        env("HTTP_ADDR", ":8080"),
		DatabaseURL:     strings.TrimSpace(os.Getenv("DATABASE_URL")),
		RedisURL:        strings.TrimSpace(os.Getenv("REDIS_URL")),
		LogLevel:        env("LOG_LEVEL", "info"),
		ShutdownTimeout: 10 * time.Second,
		SessionCacheTTL: 15 * time.Minute,
		LoginRateLimit:  10,
		LoginRateWindow: time.Minute,
		NewRelicAppName: env("NEW_RELIC_APP_NAME", "sewa-motor-backend"),
		NewRelicLicenseKey: strings.TrimSpace(
			os.Getenv("NEW_RELIC_LICENSE_KEY"),
		),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	var err error
	if cfg.AutoMigrate, err = strconv.ParseBool(env("AUTO_MIGRATE", "false")); err != nil {
		return Config{}, fmt.Errorf("AUTO_MIGRATE: %w", err)
	}
	if cfg.ShutdownTimeout, err = time.ParseDuration(env("SHUTDOWN_TIMEOUT", "10s")); err != nil {
		return Config{}, fmt.Errorf("SHUTDOWN_TIMEOUT: %w", err)
	}
	if cfg.SessionCacheTTL, err = time.ParseDuration(env("SESSION_CACHE_TTL", "15m")); err != nil {
		return Config{}, fmt.Errorf("SESSION_CACHE_TTL: %w", err)
	}
	if cfg.LoginRateLimit, err = strconv.Atoi(env("LOGIN_RATE_LIMIT", "10")); err != nil || cfg.LoginRateLimit < 1 {
		return Config{}, fmt.Errorf("LOGIN_RATE_LIMIT must be a positive integer")
	}
	if cfg.LoginRateWindow, err = time.ParseDuration(env("LOGIN_RATE_WINDOW", "1m")); err != nil {
		return Config{}, fmt.Errorf("LOGIN_RATE_WINDOW: %w", err)
	}
	newRelicDefault := strconv.FormatBool(cfg.NewRelicLicenseKey != "")
	if cfg.NewRelicEnabled, err = strconv.ParseBool(env("NEW_RELIC_ENABLED", newRelicDefault)); err != nil {
		return Config{}, fmt.Errorf("NEW_RELIC_ENABLED: %w", err)
	}
	if cfg.NewRelicEnabled && cfg.NewRelicLicenseKey == "" {
		return Config{}, fmt.Errorf("NEW_RELIC_LICENSE_KEY is required when NEW_RELIC_ENABLED=true")
	}
	if cfg.NewRelicDistributedTracing, err = strconv.ParseBool(env("NEW_RELIC_DISTRIBUTED_TRACING_ENABLED", "true")); err != nil {
		return Config{}, fmt.Errorf("NEW_RELIC_DISTRIBUTED_TRACING_ENABLED: %w", err)
	}
	if cfg.NewRelicLogForwarding, err = strconv.ParseBool(env("NEW_RELIC_LOG_FORWARDING_ENABLED", "true")); err != nil {
		return Config{}, fmt.Errorf("NEW_RELIC_LOG_FORWARDING_ENABLED: %w", err)
	}
	if proxies := strings.TrimSpace(os.Getenv("TRUSTED_PROXIES")); proxies != "" {
		for _, proxy := range strings.Split(proxies, ",") {
			if proxy = strings.TrimSpace(proxy); proxy != "" {
				cfg.TrustedProxies = append(cfg.TrustedProxies, proxy)
			}
		}
	}
	return cfg, nil
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
