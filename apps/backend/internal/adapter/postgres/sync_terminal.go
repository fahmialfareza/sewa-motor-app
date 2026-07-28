package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"gorm.io/gorm"
)

func (s *Store) TerminalIDByInstallation(ctx context.Context, installationID uuid.UUID) (*uuid.UUID, error) {
	defer observability.StartSegment(ctx, "Postgres.TerminalIDByInstallation")()
	var record terminalRecord
	err := s.ORM.WithContext(ctx).
		Select("id").
		Where("installation_id = ? AND is_active AND revoked_at IS NULL", installationID.String()).
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
	var existingID uuid.UUID
	var existingKey []byte
	var active bool
	err = tx.QueryRow(ctx, `
		SELECT id, public_key, is_active AND revoked_at IS NULL
		FROM terminals WHERE installation_id = $1
		FOR UPDATE`,
		input.InstallationID,
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
				id, installation_id, name, public_key, device_model, os_version, app_version, enrolled_by
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			existingID, input.InstallationID, input.Name, input.PublicKey,
			input.DeviceModel, input.OSVersion, input.AppVersion, principal.UserID,
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
	terminal, err := terminalByID(ctx, tx, existingID)
	if err != nil {
		return domain.Terminal{}, dbError(err, "read enrolled terminal")
	}
	identity := principalIdentity(principal)
	identity.TerminalID = &existingID
	if err = audit(ctx, tx, "terminal.enrolled", "terminal", existingID.String(), identity, nil, terminal, nil, s.Now()); err != nil {
		return domain.Terminal{}, dbError(err, "audit terminal enrollment")
	}
	// Terminals are durable server state and must be included in subsequent pulls.
	body, marshalErr := json.Marshal(terminal)
	if marshalErr != nil {
		return domain.Terminal{}, domain.WrapInternal(marshalErr, "marshal terminal change")
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO sync_changes (aggregate, aggregate_id, action, payload)
		VALUES ('terminal',$1,'created',$2)`,
		existingID.String(), body,
	); err != nil {
		// Initial migration's aggregate constraint may predate terminal support;
		// keep enrollment correct even if a development database has not migrated.
		return domain.Terminal{}, dbError(err, "sync terminal enrollment")
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Terminal{}, dbError(err, "commit terminal enrollment")
	}
	return terminal, nil
}

func (s *Store) GetTerminal(ctx context.Context, id uuid.UUID) (domain.Terminal, error) {
	defer observability.StartSegment(ctx, "Postgres.GetTerminal")()
	var record terminalRecord
	err := s.ORM.WithContext(ctx).Where("id = ?", id).Take(&record).Error
	return record.domainTerminal(), dbError(err, "get terminal")
}

func (s *Store) RevokeTerminal(ctx context.Context, principal domain.Principal, id uuid.UUID) (domain.Terminal, error) {
	defer observability.StartSegment(ctx, "Postgres.RevokeTerminal")()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.Terminal{}, dbError(err, "begin terminal revocation")
	}
	defer tx.Rollback(ctx)
	before, err := terminalByID(ctx, tx, id)
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
	after, err := terminalByID(ctx, tx, id)
	if err != nil {
		return domain.Terminal{}, dbError(err, "read revoked terminal")
	}
	identity := principalIdentity(principal)
	if err = audit(ctx, tx, "terminal.revoked", "terminal", id.String(), identity, before, after, nil, s.Now()); err != nil {
		return domain.Terminal{}, dbError(err, "audit terminal revocation")
	}
	if err = addChange(ctx, tx, "terminal", id.String(), "deleted", nil, after, true); err != nil {
		return domain.Terminal{}, dbError(err, "sync terminal revocation")
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Terminal{}, dbError(err, "commit terminal revocation")
	}
	return after, nil
}

func (s *Store) TerminalPublicKey(ctx context.Context, terminalID uuid.UUID) ([]byte, error) {
	defer observability.StartSegment(ctx, "Postgres.TerminalPublicKey")()
	var publicKey []byte
	err := s.Pool.QueryRow(ctx, `
		SELECT public_key FROM terminals
		WHERE id = $1 AND is_active AND revoked_at IS NULL`,
		terminalID,
	).Scan(&publicKey)
	return publicKey, dbError(err, "get terminal public key")
}

func (s *Store) OriginSessionMatches(ctx context.Context, sessionID, actorID, terminalID uuid.UUID) (bool, error) {
	defer observability.StartSegment(ctx, "Postgres.OriginSessionMatches")()
	var matches bool
	err := s.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM sessions
			WHERE id = $1 AND user_id = $2 AND terminal_id = $3
		)`,
		sessionID, actorID, terminalID,
	).Scan(&matches)
	return matches, dbError(err, "validate origin session")
}

func (s *Store) PullChanges(ctx context.Context, cursor int64, limit int) ([]domain.SyncChange, error) {
	defer observability.StartSegment(ctx, "Postgres.PullChanges")()
	rows, err := s.Pool.Query(ctx, `
		SELECT cursor, aggregate, aggregate_id,
		       CASE WHEN action = 'deleted' THEN 'delete' ELSE 'upsert' END,
		       revision, payload, tombstone, created_at
		FROM sync_changes
		WHERE cursor > $1
		ORDER BY cursor
		LIMIT $2`,
		cursor, limit,
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
		); err != nil {
			return nil, dbError(err, "scan sync change")
		}
		changes = append(changes, change)
	}
	return changes, dbError(rows.Err(), "iterate sync changes")
}

func (s *Store) GetOperationResult(ctx context.Context, terminalID uuid.UUID, operationID string) (*domain.StoredOperationResult, error) {
	defer observability.StartSegment(ctx, "Postgres.GetOperationResult")()
	var result domain.StoredOperationResult
	err := s.Pool.QueryRow(ctx, `
		SELECT request_hash, response_status, response
		FROM idempotency_records
		WHERE terminal_id = $1 AND operation_id = $2`,
		terminalID, operationID,
	).Scan(&result.RequestHash, &result.Status, &result.Response)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, dbError(err, "get operation result")
	}
	return &result, nil
}

func (s *Store) StoreOperationResult(ctx context.Context, terminalID uuid.UUID, operationID string, requestHash []byte, status int, response json.RawMessage) error {
	defer observability.StartSegment(ctx, "Postgres.StoreOperationResult")()
	_, err := s.Pool.Exec(ctx, `
		INSERT INTO idempotency_records (
			terminal_id, operation_id, request_hash, response_status, response
		) VALUES ($1,$2,$3,$4,$5)`,
		terminalID, operationID, requestHash, status, response,
	)
	return dbError(err, "store operation result")
}

func terminalByID(ctx context.Context, query rowQuerier, id uuid.UUID) (domain.Terminal, error) {
	defer observability.StartSegment(ctx, "Postgres.terminalByID")()
	var terminal domain.Terminal
	err := query.QueryRow(ctx, `
		SELECT id, installation_id, name, public_key, 'Ed25519', platform,
		       device_model, os_version, app_version, is_active, created_at, revoked_at
		FROM terminals WHERE id = $1`,
		id,
	).Scan(
		&terminal.ID, &terminal.InstallationID, &terminal.Name, &terminal.PublicKey,
		&terminal.Algorithm, &terminal.Platform, &terminal.DeviceModel, &terminal.OSVersion,
		&terminal.AppVersion, &terminal.IsActive, &terminal.CreatedAt, &terminal.RevokedAt,
	)
	return terminal, err
}
