package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/port"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,63}$`)

type Manifest struct {
	Users []ManifestUser `json:"users"`
}

type ManifestUser struct {
	FullName          string      `json:"fullName"`
	Username          string      `json:"username"`
	Role              domain.Role `json:"role"`
	TemporaryPassword string      `json:"temporaryPassword"`
}

func Parse(body []byte) (Manifest, error) {
	var manifest Manifest
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode bootstrap manifest: %w", err)
	}
	if len(manifest.Users) != 8 {
		return Manifest{}, fmt.Errorf("bootstrap manifest must contain exactly eight users")
	}
	seen := make(map[string]struct{}, len(manifest.Users))
	superadmins := 0
	for index := range manifest.Users {
		user := &manifest.Users[index]
		if err := normalizeAndValidateUser(user, index); err != nil {
			return Manifest{}, err
		}
		if _, duplicate := seen[user.Username]; duplicate {
			return Manifest{}, fmt.Errorf("duplicate bootstrap username %q", user.Username)
		}
		seen[user.Username] = struct{}{}
		if user.Role == domain.RoleSuperadmin {
			superadmins++
		}
	}
	if superadmins != 1 {
		return Manifest{}, fmt.Errorf("bootstrap manifest must contain exactly one superadmin and seven admins")
	}
	return manifest, nil
}

// NewSampleSuperadminManifest validates one development-only sample account
// without weakening the eight-user production bootstrap contract in Parse.
func NewSampleSuperadminManifest(fullName, username, temporaryPassword string) (Manifest, error) {
	user := ManifestUser{
		FullName:          fullName,
		Username:          username,
		Role:              domain.RoleSuperadmin,
		TemporaryPassword: temporaryPassword,
	}
	if err := normalizeAndValidateUser(&user, 0); err != nil {
		return Manifest{}, err
	}
	return Manifest{Users: []ManifestUser{user}}, nil
}

func normalizeAndValidateUser(user *ManifestUser, index int) error {
	user.FullName = strings.TrimSpace(user.FullName)
	user.Username = domain.NormalizeUsername(user.Username)
	if user.FullName == "" ||
		len(user.FullName) > 160 ||
		!usernamePattern.MatchString(user.Username) ||
		!user.Role.Valid() {
		return fmt.Errorf("bootstrap user %d is invalid", index)
	}
	if err := domain.ValidatePassword(user.TemporaryPassword); err != nil {
		return fmt.Errorf("bootstrap user %q password: %w", user.Username, err)
	}
	return nil
}

// Apply is idempotent by username. Existing users are verified but never have
// their credentials silently overwritten.
func Apply(ctx context.Context, pool *pgxpool.Pool, passwords port.PasswordHasher, manifest Manifest) (int, error) {
	return apply(
		ctx,
		func(ctx context.Context, options pgx.TxOptions) (bootstrapTx, error) {
			return pool.BeginTx(ctx, options)
		},
		passwords,
		manifest,
	)
}

func apply(
	ctx context.Context,
	begin beginBootstrapTx,
	passwords port.PasswordHasher,
	manifest Manifest,
) (int, error) {
	tx, err := begin(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return 0, fmt.Errorf("begin bootstrap: %w", err)
	}
	defer tx.Rollback(ctx)
	inserted := 0
	for _, user := range manifest.Users {
		var existingRole domain.Role
		var existingName string
		err = tx.QueryRow(ctx,
			`SELECT role, full_name FROM users WHERE username = $1`,
			user.Username,
		).Scan(&existingRole, &existingName)
		if err == nil {
			if existingRole != user.Role || existingName != user.FullName {
				return 0, fmt.Errorf("existing user %q does not match manifest identity", user.Username)
			}
			continue
		}
		if err != pgx.ErrNoRows {
			return 0, fmt.Errorf("find bootstrap user %q: %w", user.Username, err)
		}
		hash, err := passwords.Hash(user.TemporaryPassword)
		if err != nil {
			return 0, fmt.Errorf("hash bootstrap password for %q: %w", user.Username, err)
		}
		id := uuid.New()
		if _, err = tx.Exec(ctx, `
			INSERT INTO users (
				id, full_name, username, password_hash, role, is_active, must_change_password
			) VALUES ($1,$2,$3,$4,$5,true,true)`,
			id, user.FullName, user.Username, hash, user.Role,
		); err != nil {
			return 0, fmt.Errorf("insert bootstrap user %q: %w", user.Username, err)
		}
		payload, _ := json.Marshal(map[string]any{
			"id": id, "fullName": user.FullName, "username": user.Username,
			"role": user.Role, "active": true, "mustChangePassword": true,
		})
		if _, err = tx.Exec(ctx, `
			INSERT INTO audit_events (
				event_type, aggregate_type, aggregate_id, after_values, metadata, occurred_at
			) VALUES ('user.bootstrapped','user',$1,$2,'{"command":"bootstrap"}',now())`,
			id.String(), payload,
		); err != nil {
			return 0, fmt.Errorf("audit bootstrap user %q: %w", user.Username, err)
		}
		if _, err = tx.Exec(ctx, `
			INSERT INTO sync_changes (aggregate, aggregate_id, action, payload)
			VALUES ('user',$1,'created',$2)`,
			id.String(), payload,
		); err != nil {
			return 0, fmt.Errorf("sync bootstrap user %q: %w", user.Username, err)
		}
		inserted++
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit bootstrap: %w", err)
	}
	return inserted, nil
}

// ResetSampleSuperadminPassword explicitly replaces the password for the
// development sample account. Unlike Apply, this is intentionally mutating:
// callers must put it behind an explicit development-only opt-in.
func ResetSampleSuperadminPassword(
	ctx context.Context,
	pool *pgxpool.Pool,
	passwords port.PasswordHasher,
	manifest Manifest,
) error {
	return resetSampleSuperadminPassword(
		ctx,
		func(ctx context.Context, options pgx.TxOptions) (bootstrapTx, error) {
			return pool.BeginTx(ctx, options)
		},
		passwords,
		manifest,
	)
}

type bootstrapTx interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Commit(context.Context) error
	Rollback(context.Context) error
}

type beginBootstrapTx func(context.Context, pgx.TxOptions) (bootstrapTx, error)

func resetSampleSuperadminPassword(
	ctx context.Context,
	begin beginBootstrapTx,
	passwords port.PasswordHasher,
	manifest Manifest,
) error {
	if len(manifest.Users) != 1 {
		return fmt.Errorf("sample superadmin reset requires exactly one user")
	}
	user := manifest.Users[0]
	if err := normalizeAndValidateUser(&user, 0); err != nil {
		return err
	}
	if user.Role != domain.RoleSuperadmin {
		return fmt.Errorf("sample superadmin reset requires the superadmin role")
	}

	tx, err := begin(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin sample superadmin password reset: %w", err)
	}
	defer tx.Rollback(ctx)

	before, err := scanSampleUser(tx.QueryRow(ctx, `
		SELECT id, full_name, username, role, is_active, must_change_password,
		       created_at, updated_at, deleted_at
		FROM users
		WHERE username = $1
		FOR UPDATE`,
		user.Username,
	))
	if err != nil {
		return fmt.Errorf("find sample superadmin %q: %w", user.Username, err)
	}
	if before.FullName != user.FullName || before.Role != domain.RoleSuperadmin {
		return fmt.Errorf("existing user %q does not match sample superadmin identity", user.Username)
	}
	if !before.IsActive || before.DeletedAt != nil {
		return fmt.Errorf("sample superadmin %q is not active", user.Username)
	}

	passwordHash, err := passwords.Hash(user.TemporaryPassword)
	if err != nil {
		return fmt.Errorf("hash sample superadmin password: %w", err)
	}
	after, err := scanSampleUser(tx.QueryRow(ctx, `
		UPDATE users
		SET password_hash = $2, must_change_password = true, updated_at = now()
		WHERE id = $1 AND is_active AND deleted_at IS NULL
		RETURNING id, full_name, username, role, is_active, must_change_password,
		          created_at, updated_at, deleted_at`,
		before.ID, passwordHash,
	))
	if err != nil {
		return fmt.Errorf("reset sample superadmin %q password: %w", user.Username, err)
	}
	if _, err = tx.Exec(ctx, `
		UPDATE sessions
		SET revoked_at = now(), revoked_reason = 'password_reset'
		WHERE user_id = $1 AND revoked_at IS NULL`,
		before.ID,
	); err != nil {
		return fmt.Errorf("revoke sample superadmin sessions: %w", err)
	}

	beforeJSON, err := json.Marshal(before)
	if err != nil {
		return fmt.Errorf("encode sample superadmin before reset: %w", err)
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		return fmt.Errorf("encode sample superadmin after reset: %w", err)
	}
	metadataJSON, err := json.Marshal(map[string]any{
		"command":  "seed-superadmin",
		"explicit": true,
	})
	if err != nil {
		return fmt.Errorf("encode sample superadmin reset metadata: %w", err)
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO audit_events (
			event_type, aggregate_type, aggregate_id,
			before_values, after_values, metadata, occurred_at
		) VALUES ('user.password_reset','user',$1,$2,$3,$4,now())`,
		before.ID.String(), beforeJSON, afterJSON, metadataJSON,
	); err != nil {
		return fmt.Errorf("audit sample superadmin password reset: %w", err)
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO sync_changes (aggregate, aggregate_id, action, payload)
		VALUES ('user',$1,'updated',$2)`,
		after.ID.String(), afterJSON,
	); err != nil {
		return fmt.Errorf("sync sample superadmin password reset: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit sample superadmin password reset: %w", err)
	}
	return nil
}

func scanSampleUser(row pgx.Row) (domain.User, error) {
	var user domain.User
	err := row.Scan(
		&user.ID, &user.FullName, &user.Username, &user.Role, &user.IsActive,
		&user.MustChangePassword, &user.CreatedAt, &user.UpdatedAt, &user.DeletedAt,
	)
	return user, err
}
