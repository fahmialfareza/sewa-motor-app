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
	flags.StringVar(&opts.action, "action", "", "grant-superadmin, revoke-superadmin, or reset-password (required)")
	flags.StringVar(&opts.reason, "reason", "", "audited reason (required; do not include credentials)")
	flags.StringVar(&opts.operator, "operator", "", "responsible operator identity (required; recorded in audit)")
	if err := flags.Parse(args); err != nil {
		return opts, errors.New("use --username, --action grant-superadmin|revoke-superadmin|reset-password, --operator, and --reason; passwords are read only from stdin")
	}
	opts.username = domain.NormalizeUsername(opts.username)
	opts.reason = strings.TrimSpace(opts.reason)
	opts.operator = strings.TrimSpace(opts.operator)
	if opts.username == "" || len(opts.username) > 64 || opts.reason == "" || len(opts.reason) > 1000 || opts.operator == "" || len(opts.operator) > 200 || flags.NArg() != 0 {
		return opts, errors.New("an explicit username, operator identity, and reason of 1–1000 characters are required; positional arguments are not accepted")
	}
	switch opts.action {
	case "grant-superadmin", "revoke-superadmin", "reset-password":
	case "grant-platform", "revoke-platform":
		return opts, errors.New("platform permission has been removed; explicitly authorize grant-superadmin or revoke-superadmin for an organization-wide role change")
	default:
		return opts, errors.New("action must be grant-superadmin, revoke-superadmin, or reset-password")
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
	_, err = fmt.Fprintf(output, "Account operation %s completed for %s and recorded in organization audit history.\n", opts.action, opts.username)
	return err
}

func apply(ctx context.Context, store *postgres.Store, opts options, passwordHash string) error {
	defer observability.StartSegment(ctx, "Operator.AccountAdmin")()
	tx, err := store.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return errors.New("could not begin account operation")
	}
	defer tx.Rollback(ctx)
	// Use the same lock as mobile account management, before any account rows.
	// This makes concurrent last-Superadmin decisions atomic across both tools.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('organization-account-administration',0))`); err != nil {
		return errors.New("could not lock organization account administration")
	}
	var id uuid.UUID
	var active bool
	var previousRole domain.Role
	err = tx.QueryRow(ctx, `SELECT id,is_active AND deleted_at IS NULL,role FROM users WHERE username=$1 FOR UPDATE`, opts.username).Scan(&id, &active, &previousRole)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("account not found; this command does not create accounts")
	}
	if err != nil {
		return errors.New("could not lock account; apply the internal organization GORM migration before using this command")
	}
	if !active {
		return errors.New("account is inactive or deleted")
	}
	// Match the application lock order: account, then tenant (sorted), then
	// membership/data space/resource. Holding the account exclusively prevents
	// another request for this identity from entering during session revocation.
	rows, err := tx.Query(ctx, `SELECT id FROM tenants ORDER BY id FOR SHARE`)
	if err != nil {
		return errors.New("could not lock account tenants")
	}
	var tenantIDs []uuid.UUID
	for rows.Next() {
		var tenantID uuid.UUID
		if err = rows.Scan(&tenantID); err != nil {
			rows.Close()
			return errors.New("could not read account tenants")
		}
		tenantIDs = append(tenantIDs, tenantID)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return errors.New("could not read account tenants")
	}
	var revoked int64
	nextRole := previousRole
	if opts.action == "grant-superadmin" {
		nextRole = domain.RoleSuperadmin
	} else if opts.action == "revoke-superadmin" {
		nextRole = domain.RoleAdmin
		if previousRole == domain.RoleSuperadmin {
			var remaining int
			if err = tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE id<>$1 AND role='superadmin' AND is_active AND deleted_at IS NULL`, id).Scan(&remaining); err != nil {
				return errors.New("could not verify remaining Superadmins")
			}
			if remaining == 0 {
				return errors.New("cannot revoke the last active Superadmin")
			}
		}
	}
	switch opts.action {
	case "grant-superadmin", "revoke-superadmin":
		if previousRole != nextRole {
			_, err = tx.Exec(ctx, `UPDATE users SET role=$2,updated_at=now() WHERE id=$1`, id, nextRole)
		}
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
	if opts.action == "reset-password" || previousRole != nextRole {
		reason := "account_access_changed"
		if opts.action == "reset-password" {
			reason = "password_recovered"
		}
		result, e := tx.Exec(ctx, `UPDATE sessions SET revoked_at=now(),revoked_reason=$2 WHERE user_id=$1 AND revoked_at IS NULL`, id, reason)
		if e != nil {
			return errors.New("could not revoke account sessions")
		}
		revoked = result.RowsAffected()
		if err = publishAccountChange(ctx, tx, id, tenantIDs); err != nil {
			return err
		}
	}
	metadata, _ := json.Marshal(map[string]any{"source": "account-admin", "operator": opts.operator, "reason": opts.reason, "targetUserId": id, "previousRole": previousRole, "role": nextRole, "revokedSessions": revoked})
	if _, err = tx.Exec(ctx, `INSERT INTO platform_audit_events(event_type,metadata) VALUES($1,$2)`, "operator."+opts.action, metadata); err != nil {
		return errors.New("could not record account audit event")
	}
	if err = tx.Commit(ctx); err != nil {
		return errors.New("could not commit account operation")
	}
	return nil
}

// Match the API's safe account projection and tenant-generation lock. Credentials
// and historical membership permissions never enter synchronized cache records.
func publishAccountChange(ctx context.Context, tx pgx.Tx, id uuid.UUID, tenantIDs []uuid.UUID) error {
	defer observability.StartSegment(ctx, "Operator.PublishAccountChange")()
	for _, tenantID := range tenantIDs {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock_shared(hashtextextended($1,0))`, "sewa-motor-sandbox-generation:"+tenantID.String()); err != nil {
			return errors.New("could not lock account projection generation")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO sync_changes(data_space_id,aggregate,aggregate_id,action,payload)
		 SELECT ds.id,'user',u.id::text,'updated',jsonb_build_object(
		 'id',u.id,'fullName',u.full_name,'username',u.username,'role',u.role,'active',u.is_active,
		 'membershipId',m.id,'tenantId',ds.tenant_id,'mustChangePassword',u.must_change_password,
		 'createdAt',u.created_at,'updatedAt',u.updated_at,'deletedAt',u.deleted_at)
		 FROM data_spaces ds JOIN users u ON u.id=$1
		 LEFT JOIN tenant_memberships m ON m.tenant_id=ds.tenant_id AND m.user_id=u.id
		 WHERE ds.tenant_id=$2 AND ds.status='active'`, id, tenantID); err != nil {
			return errors.New("could not publish safe account projection")
		}
	}
	return nil
}
