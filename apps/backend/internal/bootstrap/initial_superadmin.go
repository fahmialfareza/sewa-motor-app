package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/port"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// InitialSuperadminInput is operator-only. Never log this value: it contains a
// temporary credential. Operator and Reason must not include credentials.
type InitialSuperadminInput struct {
	Username          string
	FullName          string
	TemporaryPassword string
	Operator          string
	Reason            string
}

func (input InitialSuperadminInput) NormalizeAndValidate() (InitialSuperadminInput, error) {
	input.Username = domain.NormalizeUsername(input.Username)
	input.FullName = strings.TrimSpace(input.FullName)
	input.Operator = strings.TrimSpace(input.Operator)
	input.Reason = strings.TrimSpace(input.Reason)
	if !usernamePattern.MatchString(input.Username) || input.FullName == "" || len(input.FullName) > 160 {
		return input, errors.New("username must contain 3–64 lowercase letters, digits, '.', '_' or '-' and full name must contain 1–160 bytes")
	}
	if input.Operator == "" || len(input.Operator) > 200 || input.Reason == "" || len(input.Reason) > 1000 {
		return input, errors.New("an operator identity of 1–200 bytes and an audited reason of 1–1000 bytes are required")
	}
	// Temporary operator credentials follow account-admin's stdin contract. The
	// account must choose a new password satisfying the normal policy at login.
	if len(input.TemporaryPassword) < 8 || len(input.TemporaryPassword) > 256 || strings.ContainsAny(input.TemporaryPassword, "\r\n") || strings.TrimSpace(input.TemporaryPassword) == "" {
		return input, errors.New("temporary password must be a single nonblank line of 8–256 bytes")
	}
	return input, nil
}

// ApplyInitialSuperadmin creates exactly one first global Superadmin, without
// applying migrations or altering existing accounts. A matching, active account
// is an idempotent no-op; its password and forced-change state remain untouched.
func ApplyInitialSuperadmin(ctx context.Context, pool *pgxpool.Pool, passwords port.PasswordHasher, input InitialSuperadminInput) (bool, error) {
	return applyInitialSuperadmin(ctx, func(ctx context.Context, options pgx.TxOptions) (bootstrapTx, error) {
		return pool.BeginTx(ctx, options)
	}, passwords, input)
}

func applyInitialSuperadmin(ctx context.Context, begin beginBootstrapTx, passwords port.PasswordHasher, input InitialSuperadminInput) (bool, error) {
	defer observability.StartSegment(ctx, "Operator.BootstrapInitialSuperadmin")()
	input, err := input.NormalizeAndValidate()
	if err != nil {
		return false, err
	}
	tx, err := begin(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return false, errors.New("could not begin initial Superadmin bootstrap")
	}
	defer tx.Rollback(ctx)
	// Same ordering boundary as mobile management and the other operator tools.
	// Count and insert must be serialized to prevent concurrent first accounts.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('organization-account-administration',0))`); err != nil {
		return false, errors.New("could not lock organization account administration")
	}
	var migrated bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version='000006_internal_organization')`).Scan(&migrated); err != nil || !migrated {
		return false, errors.New("apply the current GORM migrations, including 000006_internal_organization, before bootstrapping; this command never migrates the database")
	}
	var name string
	var role domain.Role
	var active bool
	err = tx.QueryRow(ctx, `SELECT full_name,role,is_active AND deleted_at IS NULL FROM users WHERE username=$1 FOR UPDATE`, input.Username).Scan(&name, &role, &active)
	if err == nil {
		if name != input.FullName || role != domain.RoleSuperadmin || !active {
			return false, errors.New("username already exists with a different identity, role, or inactive/deleted status; bootstrap never overwrites, promotes, or reactivates accounts")
		}
		if err = tx.Commit(ctx); err != nil {
			return false, errors.New("could not finish idempotent Superadmin bootstrap")
		}
		return false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, errors.New("could not read accounts; apply the current GORM migrations before bootstrapping")
	}
	var existing bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE role='superadmin' AND is_active AND deleted_at IS NULL)`).Scan(&existing); err != nil {
		return false, errors.New("could not verify the initial Superadmin requirement")
	}
	if existing {
		return false, errors.New("an active Superadmin already exists; use Pengguna management to create staff or the audited account-admin command for recovery")
	}
	hash, err := passwords.Hash(input.TemporaryPassword)
	if err != nil {
		return false, errors.New("could not hash the temporary password")
	}
	id := uuid.New()
	if _, err = tx.Exec(ctx, `INSERT INTO users(id,full_name,username,password_hash,role,is_active,must_change_password)
	 VALUES($1,$2,$3,$4,'superadmin',true,true)`, id, input.FullName, input.Username, hash); err != nil {
		return false, errors.New("could not create initial Superadmin")
	}
	// Lock all tenants in canonical order before attribution/generation records.
	if _, err = tx.Exec(ctx, `DO $$ DECLARE tenant uuid; BEGIN
	 FOR tenant IN SELECT id FROM tenants ORDER BY id FOR SHARE LOOP NULL; END LOOP;
	 END $$`); err != nil {
		return false, errors.New("could not lock initial Superadmin tenant projections")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO tenant_memberships(tenant_id,user_id,role) VALUES($1,$2,'superadmin')`, domain.InitialTenantID(), id); err != nil {
		return false, errors.New("could not create initial tenant attribution; apply the current GORM migrations first")
	}
	if _, err = tx.Exec(ctx, lockOrganizationGenerationsSQL); err != nil {
		return false, errors.New("could not lock active synchronization generations")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO sync_changes(data_space_id,aggregate,aggregate_id,action,payload)
	 SELECT ds.id,'user',u.id::text,'created',jsonb_build_object(
	 'id',u.id,'fullName',u.full_name,'username',u.username,'role',u.role,'active',u.is_active,
	 'membershipId',m.id,'tenantId',ds.tenant_id,'mustChangePassword',u.must_change_password,
	 'createdAt',u.created_at,'updatedAt',u.updated_at,'deletedAt',u.deleted_at)
	 FROM data_spaces ds JOIN users u ON u.id=$1
	 LEFT JOIN tenant_memberships m ON m.tenant_id=ds.tenant_id AND m.user_id=u.id
	 WHERE ds.status='active'`, id); err != nil {
		return false, errors.New("could not publish safe initial Superadmin projections")
	}
	metadata, _ := json.Marshal(map[string]any{
		"source": "bootstrap-superadmin", "operator": input.Operator, "reason": input.Reason,
		"accountId": id, "role": domain.RoleSuperadmin,
	})
	if _, err = tx.Exec(ctx, `INSERT INTO platform_audit_events(event_type,metadata) VALUES('account.initial_superadmin_bootstrapped',$1)`, metadata); err != nil {
		return false, errors.New("could not record initial Superadmin audit event")
	}
	if err = tx.Commit(ctx); err != nil {
		return false, errors.New("could not commit initial Superadmin bootstrap")
	}
	return true, nil
}
