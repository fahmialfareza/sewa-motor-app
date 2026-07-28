package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Store) UserForLogin(ctx context.Context, username string) (domain.UserAuth, error) {
	defer observability.StartSegment(ctx, "Postgres.UserForLogin")()
	var record userRecord
	err := s.ORM.WithContext(ctx).
		Where("username = ?", username).
		Take(&record).Error
	return domain.UserAuth{
		User:         record.domainUser(),
		PasswordHash: record.PasswordHash,
	}, dbError(err, "find login user")
}

func (s *Store) CreateSession(ctx context.Context, userID uuid.UUID, terminalID *uuid.UUID, tokenHash []byte) (domain.Principal, error) {
	defer observability.StartSegment(ctx, "Postgres.CreateSession")()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.Principal{}, dbError(err, "begin session")
	}
	defer tx.Rollback(ctx)

	if terminalID != nil {
		var active bool
		if err = tx.QueryRow(ctx,
			`SELECT is_active AND revoked_at IS NULL FROM terminals WHERE id = $1`,
			*terminalID,
		).Scan(&active); err != nil || !active {
			if err == nil {
				err = domain.NewError(domain.CodeForbidden, "Terminal tidak aktif")
			}
			return domain.Principal{}, dbError(err, "validate terminal")
		}
	}
	var sessionID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO sessions (user_id, terminal_id, token_hash)
		VALUES ($1,$2,$3)
		RETURNING id`,
		userID, terminalID, tokenHash,
	).Scan(&sessionID)
	if err != nil {
		return domain.Principal{}, dbError(err, "create session")
	}
	principal, err := principalBySessionRow(ctx, tx, sessionID, tokenHash)
	if err != nil {
		return domain.Principal{}, dbError(err, "read created session")
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Principal{}, dbError(err, "commit session")
	}
	return principal, nil
}

func (s *Store) PrincipalByTokenHash(ctx context.Context, tokenHash []byte) (domain.Principal, error) {
	defer observability.StartSegment(ctx, "Postgres.PrincipalByTokenHash")()
	var sessionID uuid.UUID
	if err := s.Pool.QueryRow(ctx,
		`SELECT id FROM sessions WHERE token_hash = $1`,
		tokenHash,
	).Scan(&sessionID); err != nil {
		return domain.Principal{}, dbError(err, "find session token")
	}
	return s.PrincipalBySession(ctx, sessionID, tokenHash)
}

func (s *Store) PrincipalBySession(ctx context.Context, sessionID uuid.UUID, tokenHash []byte) (domain.Principal, error) {
	defer observability.StartSegment(ctx, "Postgres.PrincipalBySession")()
	principal, err := principalBySessionRow(ctx, s.Pool, sessionID, tokenHash)
	return principal, dbError(err, "authorize session")
}

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func principalBySessionRow(ctx context.Context, query rowQuerier, sessionID uuid.UUID, tokenHash []byte) (domain.Principal, error) {
	defer observability.StartSegment(ctx, "Postgres.principalBySessionRow")()
	var principal domain.Principal
	err := query.QueryRow(ctx, `
		SELECT u.id, s.id, s.terminal_id, u.full_name, u.username, u.role, u.must_change_password
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.id = $1 AND s.token_hash = $2
		  AND s.revoked_at IS NULL
		  AND u.is_active AND u.deleted_at IS NULL`,
		sessionID, tokenHash,
	).Scan(
		&principal.UserID, &principal.SessionID, &principal.TerminalID,
		&principal.FullName, &principal.Username, &principal.Role, &principal.MustChangePassword,
	)
	return principal, err
}

func (s *Store) RevokeSession(ctx context.Context, sessionID, actorID uuid.UUID, reason string) error {
	defer observability.StartSegment(ctx, "Postgres.RevokeSession")()
	tag, err := s.Pool.Exec(ctx, `
		UPDATE sessions
		SET revoked_at = now(), revoked_reason = $2
		WHERE id = $1 AND revoked_at IS NULL`,
		sessionID, reason,
	)
	if err != nil {
		return dbError(err, "revoke session")
	}
	if tag.RowsAffected() == 0 {
		return domain.NewError(domain.CodeNotFound, "Sesi tidak ditemukan")
	}
	return nil
}

func (s *Store) ChangeOwnPassword(ctx context.Context, principal domain.Principal, passwordHash string) error {
	defer observability.StartSegment(ctx, "Postgres.ChangeOwnPassword")()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return dbError(err, "begin password change")
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `
		UPDATE users
		SET password_hash = $2, must_change_password = false, updated_at = now()
		WHERE id = $1 AND is_active AND deleted_at IS NULL`,
		principal.UserID, passwordHash,
	)
	if err != nil || tag.RowsAffected() != 1 {
		if err == nil {
			err = pgx.ErrNoRows
		}
		return dbError(err, "change password")
	}
	_, err = tx.Exec(ctx, `
		UPDATE sessions
		SET revoked_at = now(), revoked_reason = 'password_changed'
		WHERE user_id = $1 AND id <> $2 AND revoked_at IS NULL`,
		principal.UserID, principal.SessionID,
	)
	if err != nil {
		return dbError(err, "revoke other sessions")
	}
	identity := principalIdentity(principal)
	if err = audit(ctx, tx, "user.password_changed", "user", principal.UserID.String(), identity, nil,
		map[string]any{"mustChangePassword": false}, nil, s.Now()); err != nil {
		return dbError(err, "audit password change")
	}
	if err = addChange(ctx, tx, "user", principal.UserID.String(), "updated", nil,
		map[string]any{"id": principal.UserID, "mustChangePassword": false}, false); err != nil {
		return dbError(err, "sync password change")
	}
	return dbError(tx.Commit(ctx), "commit password change")
}

func (s *Store) ListUsers(ctx context.Context, includeDeleted bool) ([]domain.User, error) {
	defer observability.StartSegment(ctx, "Postgres.ListUsers")()
	query := s.ORM.WithContext(ctx).Model(&userRecord{})
	if !includeDeleted {
		query = query.Where("deleted_at IS NULL")
	}
	var records []userRecord
	if err := query.Order("full_name, id").Find(&records).Error; err != nil {
		return nil, dbError(err, "list users")
	}
	users := make([]domain.User, 0, len(records))
	for _, record := range records {
		users = append(users, record.domainUser())
	}
	return users, nil
}

func (s *Store) GetUser(ctx context.Context, id uuid.UUID) (domain.User, error) {
	defer observability.StartSegment(ctx, "Postgres.GetUser")()
	var record userRecord
	err := s.ORM.WithContext(ctx).Where("id = ?", id).Take(&record).Error
	return record.domainUser(), dbError(err, "get user")
}

func (s *Store) CreateUser(ctx context.Context, actor domain.Principal, input domain.CreateUserInput, passwordHash string) (domain.User, error) {
	defer observability.StartSegment(ctx, "Postgres.CreateUser")()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.User{}, dbError(err, "begin create user")
	}
	defer tx.Rollback(ctx)
	var user domain.User
	err = tx.QueryRow(ctx, `
		INSERT INTO users (full_name, username, password_hash, role, must_change_password)
		VALUES ($1,$2,$3,$4,true)
		RETURNING id, full_name, username, role, is_active, must_change_password,
		          created_at, updated_at, deleted_at`,
		input.FullName, input.Username, passwordHash, input.Role,
	).Scan(
		&user.ID, &user.FullName, &user.Username, &user.Role, &user.IsActive,
		&user.MustChangePassword, &user.CreatedAt, &user.UpdatedAt, &user.DeletedAt,
	)
	if err != nil {
		return domain.User{}, dbError(err, "create user")
	}
	identity := principalIdentity(actor)
	if err = audit(ctx, tx, "user.created", "user", user.ID.String(), identity, nil, user, nil, s.Now()); err != nil {
		return domain.User{}, dbError(err, "audit create user")
	}
	if err = addChange(ctx, tx, "user", user.ID.String(), "created", nil, user, false); err != nil {
		return domain.User{}, dbError(err, "sync create user")
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.User{}, dbError(err, "commit create user")
	}
	return user, nil
}

func (s *Store) UpdateUser(ctx context.Context, actor domain.Principal, targetID uuid.UUID, input domain.UpdateUserInput) (domain.User, error) {
	defer observability.StartSegment(ctx, "Postgres.UpdateUser")()
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return domain.User{}, dbError(err, "begin update user")
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(83729401)`); err != nil {
		return domain.User{}, dbError(err, "lock superadmin invariant")
	}
	before, err := getUserForUpdate(ctx, tx, targetID)
	if err != nil {
		return domain.User{}, dbError(err, "lock user")
	}
	if actor.UserID == targetID {
		if input.Role != nil && *input.Role != domain.RoleSuperadmin {
			return domain.User{}, domain.NewError(domain.CodeSelfMutation, "Anda tidak dapat menurunkan peran sendiri")
		}
		if input.IsActive != nil && !*input.IsActive {
			return domain.User{}, domain.NewError(domain.CodeSelfMutation, "Anda tidak dapat menonaktifkan akun sendiri")
		}
	}
	nextRole := before.Role
	if input.Role != nil {
		nextRole = *input.Role
	}
	nextActive := before.IsActive
	if input.IsActive != nil {
		nextActive = *input.IsActive
	}
	if before.Role == domain.RoleSuperadmin && before.IsActive && before.DeletedAt == nil &&
		(nextRole != domain.RoleSuperadmin || !nextActive) {
		if err = ensureAnotherSuperadmin(ctx, tx, targetID); err != nil {
			return domain.User{}, err
		}
	}
	name := before.FullName
	if input.FullName != nil {
		name = *input.FullName
	}
	username := before.Username
	if input.Username != nil {
		username = *input.Username
	}
	var after domain.User
	err = tx.QueryRow(ctx, `
		UPDATE users
		SET full_name = $2, username = $3, role = $4, is_active = $5, updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, full_name, username, role, is_active, must_change_password,
		          created_at, updated_at, deleted_at`,
		targetID, name, username, nextRole, nextActive,
	).Scan(
		&after.ID, &after.FullName, &after.Username, &after.Role, &after.IsActive,
		&after.MustChangePassword, &after.CreatedAt, &after.UpdatedAt, &after.DeletedAt,
	)
	if err != nil {
		return domain.User{}, dbError(err, "update user")
	}
	if before.Role != after.Role || before.IsActive != after.IsActive {
		if _, err = tx.Exec(ctx, `
			UPDATE sessions SET revoked_at = now(), revoked_reason = 'user_access_changed'
			WHERE user_id = $1 AND revoked_at IS NULL`, targetID); err != nil {
			return domain.User{}, dbError(err, "revoke changed user sessions")
		}
	}
	identity := principalIdentity(actor)
	if err = audit(ctx, tx, "user.updated", "user", targetID.String(), identity, before, after, nil, s.Now()); err != nil {
		return domain.User{}, dbError(err, "audit update user")
	}
	if err = addChange(ctx, tx, "user", targetID.String(), "updated", nil, after, false); err != nil {
		return domain.User{}, dbError(err, "sync update user")
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.User{}, dbError(err, "commit update user")
	}
	return after, nil
}

func (s *Store) ResetUserPassword(ctx context.Context, actor domain.Principal, targetID uuid.UUID, passwordHash string) (domain.User, error) {
	defer observability.StartSegment(ctx, "Postgres.ResetUserPassword")()
	if actor.UserID == targetID {
		return domain.User{}, domain.NewError(domain.CodeSelfMutation, "Gunakan menu ganti kata sandi untuk akun sendiri")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.User{}, dbError(err, "begin reset password")
	}
	defer tx.Rollback(ctx)
	before, err := getUserForUpdate(ctx, tx, targetID)
	if err != nil {
		return domain.User{}, dbError(err, "lock reset user")
	}
	var after domain.User
	err = tx.QueryRow(ctx, `
		UPDATE users
		SET password_hash = $2, must_change_password = true, updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING id, full_name, username, role, is_active, must_change_password,
		          created_at, updated_at, deleted_at`,
		targetID, passwordHash,
	).Scan(
		&after.ID, &after.FullName, &after.Username, &after.Role, &after.IsActive,
		&after.MustChangePassword, &after.CreatedAt, &after.UpdatedAt, &after.DeletedAt,
	)
	if err != nil {
		return domain.User{}, dbError(err, "reset password")
	}
	if _, err = tx.Exec(ctx, `
		UPDATE sessions SET revoked_at = now(), revoked_reason = 'password_reset'
		WHERE user_id = $1 AND revoked_at IS NULL`, targetID); err != nil {
		return domain.User{}, dbError(err, "revoke reset user sessions")
	}
	identity := principalIdentity(actor)
	if err = audit(ctx, tx, "user.password_reset", "user", targetID.String(), identity, before,
		map[string]any{"mustChangePassword": true}, nil, s.Now()); err != nil {
		return domain.User{}, dbError(err, "audit password reset")
	}
	if err = addChange(ctx, tx, "user", targetID.String(), "updated", nil, after, false); err != nil {
		return domain.User{}, dbError(err, "sync password reset")
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.User{}, dbError(err, "commit password reset")
	}
	return after, nil
}

func (s *Store) DeleteUser(ctx context.Context, actor domain.Principal, targetID uuid.UUID, reason string) error {
	defer observability.StartSegment(ctx, "Postgres.DeleteUser")()
	if actor.UserID == targetID {
		return domain.NewError(domain.CodeSelfMutation, "Anda tidak dapat menghapus akun sendiri")
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return dbError(err, "begin delete user")
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(83729401)`); err != nil {
		return dbError(err, "lock superadmin invariant")
	}
	before, err := getUserForUpdate(ctx, tx, targetID)
	if err != nil {
		return dbError(err, "lock deleted user")
	}
	if before.Role == domain.RoleSuperadmin && before.IsActive && before.DeletedAt == nil {
		if err = ensureAnotherSuperadmin(ctx, tx, targetID); err != nil {
			return err
		}
	}
	var deletedAt any
	err = tx.QueryRow(ctx, `
		UPDATE users
		SET is_active = false, deleted_at = now(), updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING deleted_at`,
		targetID,
	).Scan(&deletedAt)
	if err != nil {
		return dbError(err, "delete user")
	}
	if _, err = tx.Exec(ctx, `
		UPDATE sessions SET revoked_at = now(), revoked_reason = 'user_deleted'
		WHERE user_id = $1 AND revoked_at IS NULL`, targetID); err != nil {
		return dbError(err, "revoke deleted user sessions")
	}
	identity := principalIdentity(actor)
	after := map[string]any{"id": targetID, "isActive": false, "deleted": true}
	if err = audit(ctx, tx, "user.deleted", "user", targetID.String(), identity, before, after,
		map[string]any{"reason": reason}, s.Now()); err != nil {
		return dbError(err, "audit delete user")
	}
	if err = addChange(ctx, tx, "user", targetID.String(), "deleted", nil, after, true); err != nil {
		return dbError(err, "sync delete user")
	}
	return dbError(tx.Commit(ctx), "commit delete user")
}

func getUserForUpdate(ctx context.Context, tx pgx.Tx, id uuid.UUID) (domain.User, error) {
	defer observability.StartSegment(ctx, "Postgres.getUserForUpdate")()
	var user domain.User
	err := tx.QueryRow(ctx, `
		SELECT id, full_name, username, role, is_active, must_change_password,
		       created_at, updated_at, deleted_at
		FROM users WHERE id = $1 FOR UPDATE`,
		id,
	).Scan(
		&user.ID, &user.FullName, &user.Username, &user.Role, &user.IsActive,
		&user.MustChangePassword, &user.CreatedAt, &user.UpdatedAt, &user.DeletedAt,
	)
	return user, err
}

func ensureAnotherSuperadmin(ctx context.Context, tx pgx.Tx, excluding uuid.UUID) error {
	defer observability.StartSegment(ctx, "Postgres.ensureAnotherSuperadmin")()
	var count int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM users
		WHERE role = 'superadmin' AND is_active AND deleted_at IS NULL AND id <> $1`,
		excluding,
	).Scan(&count); err != nil {
		return dbError(err, "count active superadmins")
	}
	if count < 1 {
		return domain.NewError(domain.CodeFinalSuperadmin, "Setidaknya satu superadmin aktif harus tetap tersedia")
	}
	return nil
}

func marshalJSON(value any) json.RawMessage {
	body, _ := json.Marshal(value)
	return body
}

var _ = errors.Is
