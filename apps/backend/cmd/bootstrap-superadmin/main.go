// bootstrap-superadmin creates the first global production Superadmin only.
// It never applies migrations, creates sample accounts, or resets credentials.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/adapter/security"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/bootstrap"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseOptions(args []string) (bootstrap.InitialSuperadminInput, error) {
	var input bootstrap.InitialSuperadminInput
	var confirmed bool
	flags := flag.NewFlagSet("bootstrap-superadmin", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&input.Username, "username", "", "initial account username (required)")
	flags.StringVar(&input.FullName, "full-name", "", "initial account full name (required)")
	flags.StringVar(&input.Operator, "operator", "", "responsible operator identity (audited; required)")
	flags.StringVar(&input.Reason, "reason", "", "audited reason without credentials (required)")
	flags.BoolVar(&confirmed, "confirm-production", false, "explicitly authorize initial production account creation")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return input, errors.New("use --username, --full-name, --operator, --reason, and --confirm-production; the password is accepted only through stdin")
	}
	if !confirmed {
		return input, errors.New("--confirm-production is required; verify DATABASE_URL targets the intended production database first")
	}
	// Validate nonsecret fields before consuming stdin; discard the placeholder.
	input.TemporaryPassword = "validation-only"
	normalized, err := input.NormalizeAndValidate()
	normalized.TemporaryPassword = ""
	return normalized, err
}

func readPassword(input io.Reader) (string, error) {
	body, err := io.ReadAll(io.LimitReader(input, 259))
	if err != nil {
		return "", errors.New("could not read temporary password from stdin")
	}
	password := strings.TrimSuffix(strings.TrimSuffix(string(body), "\n"), "\r")
	if len(password) < 8 || len(password) > 256 || strings.ContainsAny(password, "\r\n") || strings.TrimSpace(password) == "" {
		return "", errors.New("stdin must contain one temporary password of 8–256 bytes")
	}
	return password, nil
}

func run(args []string, input io.Reader, output io.Writer) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	// No dotenv/config fallback: this one-off command uses only the explicitly
	// injected database environment and cannot silently choose a local file.
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return errors.New("DATABASE_URL must be explicitly configured; no .env file is loaded")
	}
	opts.TemporaryPassword, err = readPassword(input)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return errors.New("could not configure the account database connection")
	}
	defer pool.Close()
	if err = pool.Ping(ctx); err != nil {
		return errors.New("could not connect to the account database; verify the intended DATABASE_URL and network access")
	}
	created, err := bootstrap.ApplyInitialSuperadmin(ctx, pool, security.DefaultArgon2id(), opts)
	if err != nil {
		return err
	}
	if !created {
		_, err = fmt.Fprintln(output, "Matching active Superadmin already exists. No credentials or account settings were changed.")
	} else {
		_, err = fmt.Fprintln(output, "Created one initial Superadmin and recorded the operator audit. Change the temporary password on first login, then create other staff through Pengguna.")
	}
	return err
}
