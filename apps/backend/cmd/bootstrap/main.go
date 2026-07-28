package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/adapter/postgres"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/adapter/security"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/bootstrap"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/config"
)

func main() {
	manifestPath := flag.String("manifest", "bootstrap-users.json", "path to the uncommitted bootstrap secret manifest")
	flag.Parse()
	body, err := os.ReadFile(*manifestPath)
	if err != nil {
		slog.Error("read bootstrap manifest", "error", err)
		os.Exit(1)
	}
	manifest, err := bootstrap.Parse(body)
	if err != nil {
		slog.Error("validate bootstrap manifest", "error", err)
		os.Exit(1)
	}
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	store, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect postgres", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	inserted, err := bootstrap.Apply(ctx, store.Pool, security.DefaultArgon2id(), manifest)
	if err != nil {
		slog.Error("bootstrap users", "error", err)
		os.Exit(1)
	}
	slog.Info("bootstrap complete", "inserted", inserted, "unchanged", len(manifest.Users)-inserted)
}
