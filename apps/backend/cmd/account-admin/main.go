// account-admin performs explicitly requested, audited global-account operations.
// Tenant memberships are intentionally not modified by this operator command.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/adapter/postgres"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/adapter/security"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/config"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
)

type options struct{ username, action, reason, operator string }

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseOptions(args []string) (options, error) {
	var opts options
	flags := flag.NewFlagSet("account-admin", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.username, "username", "", "existing account username (required)")
	flags.StringVar(&opts.action, "action", "", "grant-platform, revoke-platform, or reset-password (required)")
	flags.StringVar(&opts.reason, "reason", "", "audited reason (required; do not include credentials)")
	flags.StringVar(&opts.operator, "operator", "", "responsible operator identity (required; recorded in audit)")
	if err := flags.Parse(args); err != nil {
		return opts, errors.New("use --username, --action grant-platform|revoke-platform|reset-password, --operator, and --reason; passwords are read only from stdin")
	}
	opts.username = domain.NormalizeUsername(opts.username)
	opts.reason = strings.TrimSpace(opts.reason)
	opts.operator = strings.TrimSpace(opts.operator)
	if opts.username == "" || len(opts.username) > 64 || opts.reason == "" || len(opts.reason) > 1000 || opts.operator == "" || len(opts.operator) > 200 || flags.NArg() != 0 {
		return opts, errors.New("an explicit username, operator identity, and reason of 1–1000 characters are required; positional arguments are not accepted")
	}
	switch opts.action {
	case "grant-platform", "revoke-platform", "reset-password":
	default:
		return opts, errors.New("action must be grant-platform, revoke-platform, or reset-password")
	}
	return opts, nil
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
	var passwordHash string
	if opts.action == "reset-password" {
		password, e := readPassword(input)
		if e != nil {
			return e
		}
		passwordHash, e = security.DefaultArgon2id().Hash(password)
		if e != nil {
			return errors.New("could not hash temporary password")
		}
	}
	if err = godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("could not load backend environment")
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	store, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return errors.New("could not connect to account database")
	}
	defer store.Close()
	if err = apply(ctx, store, opts, passwordHash); err != nil {
		return err
	}
	_, err = fmt.Fprintf(output, "Account operation %s completed for %s and recorded in platform audit history.\n", opts.action, opts.username)
	return err
}

func apply(ctx context.Context, store *postgres.Store, opts options, passwordHash string) error {
	defer observability.StartSegment(ctx, "Operator.AccountAdmin")()
	tx, err := store.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return errors.New("could not begin account operation")
	}
	defer tx.Rollback(ctx)
	var id uuid.UUID
	var active, wasPlatformAdmin bool
	err = tx.QueryRow(ctx, `SELECT id,is_active AND deleted_at IS NULL,is_platform_admin FROM users WHERE username=$1 FOR UPDATE`, opts.username).Scan(&id, &active, &wasPlatformAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("account not found; this command does not create accounts")
	}
	if err != nil {
		return errors.New("could not lock account; apply the tenant GORM migration before using this command")
	}
	if !active {
		return errors.New("account is inactive or deleted")
	}
	// Match the application lock order: account, then tenant (sorted), then
	// membership/data space/resource. Holding the account exclusively prevents
	// another request for this identity from entering during session revocation.
	rows, err := tx.Query(ctx, `SELECT t.id FROM tenants t WHERE EXISTS(SELECT 1 FROM tenant_memberships m WHERE m.tenant_id=t.id AND m.user_id=$1) ORDER BY t.id FOR SHARE OF t`, id)
	if err != nil {
		return errors.New("could not lock account tenants")
	}
	for rows.Next() {
		var tenantID uuid.UUID
		if err = rows.Scan(&tenantID); err != nil {
			rows.Close()
			return errors.New("could not read account tenants")
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return errors.New("could not read account tenants")
	}
	var revoked int64
	switch opts.action {
	case "grant-platform", "revoke-platform":
		_, err = tx.Exec(ctx, `UPDATE users SET is_platform_admin=$2,updated_at=now() WHERE id=$1`, id, opts.action == "grant-platform")
	case "reset-password":
		if passwordHash == "" {
			return errors.New("a hashed temporary password is required")
		}
		_, err = tx.Exec(ctx, `UPDATE users SET password_hash=$2,must_change_password=true,updated_at=now() WHERE id=$1`, id, passwordHash)
	default:
		return errors.New("unsupported account operation")
	}
	if err != nil {
		return errors.New("could not update account")
	}
	if opts.action == "reset-password" || opts.action == "revoke-platform" {
		query := `UPDATE sessions SET revoked_at=now(),revoked_reason='operator_password_reset' WHERE user_id=$1 AND revoked_at IS NULL`
		if opts.action == "revoke-platform" {
			query = `UPDATE sessions SET revoked_at=now(),revoked_reason='platform_access_revoked' WHERE user_id=$1 AND context_kind='platform' AND revoked_at IS NULL`
		}
		result, e := tx.Exec(ctx, query, id)
		if e != nil {
			return errors.New("could not revoke account sessions")
		}
		revoked = result.RowsAffected()
	}
	metadata, _ := json.Marshal(map[string]any{"source": "account-admin", "operator": opts.operator, "reason": opts.reason, "targetUserId": id, "previousPlatformAdmin": wasPlatformAdmin, "revokedSessions": revoked})
	if _, err = tx.Exec(ctx, `INSERT INTO platform_audit_events(event_type,metadata) VALUES($1,$2)`, "operator."+opts.action, metadata); err != nil {
		return errors.New("could not record account audit event")
	}
	if err = tx.Commit(ctx); err != nil {
		return errors.New("could not commit account operation")
	}
	return nil
}
