package postgres

import (
	"bytes"
	"context"
	"errors"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"gorm.io/gorm"
)

func (s *Store) TerminalIDByInstallation(ctx context.Context, tenantID, installationID uuid.UUID) (*uuid.UUID, error) {
	defer observability.StartSegment(ctx, "Postgres.TerminalIDByInstallation")()
	var record terminalRecord
	err := s.ORM.WithContext(ctx).
		Select("id").
		Where("tenant_id = ? AND installation_id = ? AND is_active AND revoked_at IS NULL", tenantID, installationID.String()).
		Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, dbError(err, "resolve terminal installation")
	}
	return &record.ID, nil
}

func (s *Store) EnrollTerminal(ctx context.Context, principal domain.Principal, input domain.EnrollTerminalInput) (domain.Terminal, error) {
	defer observability.StartSegment(ctx, "Postgres.EnrollTerminal")()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.Terminal{}, dbError(err, "begin terminal enrollment")
	}
	defer tx.Rollback(ctx)
	if _, err = lockTenantLifecycleAccess(ctx, tx, principal); err != nil {
		return domain.Terminal{}, err
	}
	space, err := lockActiveDataSpace(ctx, tx, principal.DataSpaceID)
	if err != nil {
		return domain.Terminal{}, err
	}
	if space.Mode != domain.DataModeProduction {
		return domain.Terminal{}, domain.NewError(domain.CodeForbidden, "Pendaftaran terminal hanya tersedia di mode produksi")
	}
	var existingID uuid.UUID
	var existingKey []byte
	var active bool
	err = tx.QueryRow(ctx, `
		SELECT id, public_key, is_active AND revoked_at IS NULL
		FROM terminals WHERE installation_id = $1 AND tenant_id = $2
		FOR UPDATE`,
		input.InstallationID, principal.TenantID,
	).Scan(&existingID, &existingKey, &active)
	switch {
	case err == nil:
		if !active {
			return domain.Terminal{}, domain.NewError(domain.CodeConflict, "Terminal ini telah dicabut")
		}
		if !bytes.Equal(existingKey, input.PublicKey) {
			return domain.Terminal{}, domain.NewError(domain.CodeConflict, "Installation ID telah terdaftar dengan kunci berbeda")
		}
		if _, err = tx.Exec(ctx,
			`UPDATE terminals SET name = $2, updated_at = now() WHERE id = $1`,
			existingID, input.Name,
		); err != nil {
			return domain.Terminal{}, dbError(err, "update enrolled terminal")
		}
	case errors.Is(err, pgx.ErrNoRows):
		existingID = uuid.New()
		if _, err = tx.Exec(ctx, `
			INSERT INTO terminals (
				id, installation_id, name, public_key, device_model, os_version, app_version, enrolled_by, tenant_id
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			existingID, input.InstallationID, input.Name, input.PublicKey,
			input.DeviceModel, input.OSVersion, input.AppVersion, principal.UserID, principal.TenantID,
		); err != nil {
			return domain.Terminal{}, dbError(err, "insert terminal")
		}
	default:
		return domain.Terminal{}, dbError(err, "find enrolled terminal")
	}
	if _, err = tx.Exec(ctx,
		`UPDATE sessions SET terminal_id = $2 WHERE id = $1 AND revoked_at IS NULL`,
		principal.SessionID, existingID,
	); err != nil {
		return domain.Terminal{}, dbError(err, "bind enrolling session")
	}
	terminal, err := terminalByID(ctx, tx, principal.TenantID, existingID)
	if err != nil {
		return domain.Terminal{}, dbError(err, "read enrolled terminal")
	}
	identity := principalIdentity(principal)
	identity.TerminalID = &existingID
	if err = audit(ctx, tx, "terminal.enrolled", "terminal", existingID.String(), identity, nil, terminal, nil, s.Now()); err != nil {
		return domain.Terminal{}, dbError(err, "audit terminal enrollment")
	}
	// Terminals are shared durable state and must be fanned out to each active
	// production/Sandbox data plane.
	if err = addSharedChange(ctx, tx, "terminal", existingID.String(), "created", nil, terminal, false); err != nil {
		return domain.Terminal{}, dbError(err, "sync terminal enrollment")
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Terminal{}, dbError(err, "commit terminal enrollment")
	}
	return terminal, nil
}

func (s *Store) GetTerminal(ctx context.Context, tenantID, id uuid.UUID) (domain.Terminal, error) {
	defer observability.StartSegment(ctx, "Postgres.GetTerminal")()
	var record terminalRecord
	err := s.ORM.WithContext(ctx).Where("id = ? AND tenant_id = ?", id, tenantID).Take(&record).Error
	return record.domainTerminal(), dbError(err, "get terminal")
}

func (s *Store) RevokeTerminal(ctx context.Context, principal domain.Principal, id uuid.UUID) (domain.Terminal, error) {
	defer observability.StartSegment(ctx, "Postgres.RevokeTerminal")()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.Terminal{}, dbError(err, "begin terminal revocation")
	}
	defer tx.Rollback(ctx)
	role, err := lockTenantLifecycleAccess(ctx, tx, principal)
	if err != nil {
		return domain.Terminal{}, err
	}
	if role != domain.RoleSuperadmin {
		return domain.Terminal{}, domain.NewError(domain.CodeForbidden, "Hanya superadmin usaha dapat mencabut terminal")
	}
	space, err := lockActiveDataSpace(ctx, tx, principal.DataSpaceID)
	if err != nil {
		return domain.Terminal{}, err
	}
	if space.Mode != domain.DataModeProduction {
		return domain.Terminal{}, domain.NewError(domain.CodeForbidden, "Pencabutan terminal hanya tersedia di mode produksi")
	}
	before, err := terminalByID(ctx, tx, principal.TenantID, id)
	if err != nil {
		return domain.Terminal{}, dbError(err, "get revoked terminal")
	}
	if !before.IsActive {
		return domain.Terminal{}, domain.NewError(domain.CodeConflict, "Terminal sudah dicabut")
	}
	if _, err = tx.Exec(ctx, `
		UPDATE terminals
		SET is_active = false, revoked_at = now(), updated_at = now()
		WHERE id = $1`, id); err != nil {
		return domain.Terminal{}, dbError(err, "revoke terminal")
	}
	if _, err = tx.Exec(ctx, `
		UPDATE sessions
		SET revoked_at = now(), revoked_reason = 'terminal_revoked'
		WHERE terminal_id = $1 AND revoked_at IS NULL`, id); err != nil {
		return domain.Terminal{}, dbError(err, "revoke terminal sessions")
	}
	after, err := terminalByID(ctx, tx, principal.TenantID, id)
	if err != nil {
		return domain.Terminal{}, dbError(err, "read revoked terminal")
	}
	identity := principalIdentity(principal)
	if err = audit(ctx, tx, "terminal.revoked", "terminal", id.String(), identity, before, after, nil, s.Now()); err != nil {
		return domain.Terminal{}, dbError(err, "audit terminal revocation")
	}
	if err = addSharedChange(ctx, tx, "terminal", id.String(), "deleted", nil, after, true); err != nil {
		return domain.Terminal{}, dbError(err, "sync terminal revocation")
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Terminal{}, dbError(err, "commit terminal revocation")
	}
	return after, nil
}

func (s *Store) TerminalPublicKey(ctx context.Context, tenantID, terminalID uuid.UUID) ([]byte, error) {
	defer observability.StartSegment(ctx, "Postgres.TerminalPublicKey")()
	var publicKey []byte
	err := s.Pool.QueryRow(ctx, `
		SELECT public_key FROM terminals
		WHERE id = $1 AND tenant_id = $2 AND is_active AND revoked_at IS NULL`,
		terminalID, tenantID,
	).Scan(&publicKey)
	return publicKey, dbError(err, "get terminal public key")
}

func (s *Store) OriginSessionMatches(ctx context.Context, sessionID, actorID, terminalID, dataSpaceID uuid.UUID) (bool, error) {
	defer observability.StartSegment(ctx, "Postgres.OriginSessionMatches")()
	var matches bool
	err := s.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM sessions s
			JOIN tenant_memberships m ON m.id = s.membership_id AND m.tenant_id = s.tenant_id AND m.user_id = s.user_id
			JOIN tenants t ON t.id = s.tenant_id
			JOIN users u ON u.id = s.user_id
			WHERE s.id = $1 AND s.user_id = $2 AND s.terminal_id = $3 AND s.data_space_id = $4
			  AND s.context_kind = 'tenant' AND m.status = 'active' AND t.status = 'active'
			  AND u.is_active AND u.deleted_at IS NULL
		)`,
		sessionID, actorID, terminalID, dataSpaceID,
	).Scan(&matches)
	return matches, dbError(err, "validate origin session")
}

func (s *Store) PullChanges(ctx context.Context, dataSpaceID uuid.UUID, cursor int64, limit int) ([]domain.SyncChange, error) {
	defer observability.StartSegment(ctx, "Postgres.PullChanges")()
	rows, err := s.Pool.Query(ctx, `
		SELECT cursor, aggregate, aggregate_id,
		       CASE WHEN action = 'deleted' THEN 'delete' ELSE 'upsert' END,
		       revision, payload, tombstone, created_at, data_space_id
		FROM sync_changes
		WHERE data_space_id = $1 AND cursor > $2
		ORDER BY cursor
		LIMIT $3`,
		dataSpaceID, cursor, limit,
	)
	if err != nil {
		return nil, dbError(err, "pull sync changes")
	}
	defer rows.Close()
	changes := make([]domain.SyncChange, 0)
	for rows.Next() {
		var change domain.SyncChange
		if err := rows.Scan(
			&change.Cursor, &change.Aggregate, &change.AggregateID, &change.Action,
			&change.Revision, &change.Payload, &change.Tombstone, &change.CreatedAt,
			&change.DataSpaceID,
		); err != nil {
			return nil, dbError(err, "scan sync change")
		}
		changes = append(changes, change)
	}
	return changes, dbError(rows.Err(), "iterate sync changes")
}

func terminalByID(ctx context.Context, query rowQuerier, tenantID, id uuid.UUID) (domain.Terminal, error) {
	defer observability.StartSegment(ctx, "Postgres.terminalByID")()
	var terminal domain.Terminal
	err := query.QueryRow(ctx, `
		SELECT id, installation_id, name, public_key, 'Ed25519', platform,
		       device_model, os_version, app_version, is_active, created_at, revoked_at
		FROM terminals WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	).Scan(
		&terminal.ID, &terminal.InstallationID, &terminal.Name, &terminal.PublicKey,
		&terminal.Algorithm, &terminal.Platform, &terminal.DeviceModel, &terminal.OSVersion,
		&terminal.AppVersion, &terminal.IsActive, &terminal.CreatedAt, &terminal.RevokedAt,
	)
	return terminal, err
}
