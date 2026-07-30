package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/adapter/postgres"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/adapter/security"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/bootstrap"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/config"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/migrations"
	"github.com/joho/godotenv"
)

const defaultDevelopmentPassword = "superadmin123"

type options struct {
	resetPassword bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("seed sample superadmin", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	options, err := parseOptions(args)
	if err != nil {
		return err
	}
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load backend .env: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "development") {
		return errors.New("sample superadmin seeding requires APP_ENV=development")
	}

	password, usingDefaultPassword := developmentPassword()
	manifest, err := bootstrap.NewSampleSuperadminManifest(
		env("DEV_SUPERADMIN_FULL_NAME", "Penyok"),
		env("DEV_SUPERADMIN_USERNAME", "superadmin"),
		password,
	)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	store, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := migrations.Apply(ctx, store.ORM); err != nil {
		return err
	}
	inserted, err := bootstrap.Apply(ctx, store.Pool, security.DefaultArgon2id(), manifest)
	if err != nil {
		return err
	}

	user := manifest.Users[0]
	if inserted == 0 {
		if options.resetPassword {
			if err := bootstrap.ResetSampleSuperadminPassword(
				ctx,
				store.Pool,
				security.DefaultArgon2id(),
				manifest,
			); err != nil {
				return err
			}
			fmt.Printf("Sample superadmin password reset.\nUsername: %s\n", user.Username)
			printTemporaryPassword(password, usingDefaultPassword)
			fmt.Println("The account must change its temporary password after the next login.")
			return nil
		}
		fmt.Printf(
			"Sample superadmin %q already exists; its password was not changed.\n",
			user.Username,
		)
		return nil
	}
	fmt.Printf("Sample superadmin created.\nUsername: %s\n", user.Username)
	printTemporaryPassword(password, usingDefaultPassword)
	fmt.Println("The account must change its temporary password after the first login.")
	return nil
}

func parseOptions(args []string) (options, error) {
	flags := flag.NewFlagSet("seed-superadmin", flag.ContinueOnError)
	var parsed options
	flags.BoolVar(
		&parsed.resetPassword,
		"reset-password",
		false,
		"replace an existing sample superadmin password and revoke its sessions",
	)
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if flags.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	return parsed, nil
}

func printTemporaryPassword(password string, usingDefault bool) {
	if usingDefault {
		fmt.Printf("Temporary password: %s\n", password)
		return
	}
	fmt.Println("Temporary password: loaded from DEV_SUPERADMIN_PASSWORD")
}

func developmentPassword() (string, bool) {
	password := strings.TrimSpace(os.Getenv("DEV_SUPERADMIN_PASSWORD"))
	if password != "" {
		return password, false
	}
	return defaultDevelopmentPassword, true
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
