package postgres

import (
	"context"
	"encoding/json"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/jackc/pgx/v5"
)

func (s *Store) RecordPrintAttempt(ctx context.Context, input domain.PrintAttemptInput) (domain.PrintAttempt, error) {
	defer observability.StartSegment(ctx, "Postgres.RecordPrintAttempt")()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.PrintAttempt{}, dbError(err, "begin print attempt")
	}
	defer tx.Rollback(ctx)
	attempt, err := s.recordPrintAttemptTx(ctx, tx, input)
	if err != nil {
		return domain.PrintAttempt{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.PrintAttempt{}, dbError(err, "commit print attempt")
	}
	return attempt, nil
}

func (s *Store) recordPrintAttemptTx(ctx context.Context, tx pgx.Tx, input domain.PrintAttemptInput) (domain.PrintAttempt, error) {
	defer observability.StartSegment(ctx, "Postgres.recordPrintAttemptTx")()
	if len(input.Metadata) == 0 {
		input.Metadata = json.RawMessage(`{}`)
	}
	var attempt domain.PrintAttempt
	err := tx.QueryRow(ctx, `
		INSERT INTO print_attempts (
			id, transaction_id, transaction_revision, terminal_id, actor_id, session_id,
			status, is_copy, printer_kind, printer_identifier, error_code, error_message,
			metadata, client_occurred_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING id, transaction_id, transaction_revision, terminal_id, status, is_copy,
		          actor_id, printer_kind, printer_identifier, error_code, error_message, metadata,
		          client_occurred_at, server_received_at`,
		input.ID, input.TransactionID, input.Revision, input.Identity.TerminalID,
		input.Identity.OriginActorID, input.Identity.OriginSessionID,
		input.Status, input.IsCopy, input.PrinterKind, input.PrinterIdentifier,
		input.ErrorCode, input.ErrorMessage, input.Metadata, input.OccurredAt,
	).Scan(
		&attempt.ID, &attempt.TransactionID, &attempt.TransactionRevision, &attempt.TerminalID,
		&attempt.Status, &attempt.IsCopy, &attempt.ActorID, &attempt.PrinterKind, &attempt.PrinterIdentifier,
		&attempt.ErrorCode, &attempt.ErrorMessage, &attempt.Metadata,
		&attempt.ClientOccurredAt, &attempt.ServerReceivedAt,
	)
	if err != nil {
		return domain.PrintAttempt{}, dbError(err, "insert print attempt")
	}
	var currentRevision int
	if err = tx.QueryRow(ctx,
		`SELECT current_revision FROM transactions WHERE id = $1 FOR UPDATE`,
		input.TransactionID,
	).Scan(&currentRevision); err != nil {
		return domain.PrintAttempt{}, dbError(err, "lock printed transaction")
	}
	if currentRevision == input.Revision {
		switch input.Status {
		case "success":
			_, err = tx.Exec(ctx, `
				UPDATE transactions
				SET print_state = 'success', latest_printed_revision = $2, updated_at = now()
				WHERE id = $1`, input.TransactionID, input.Revision)
		case "failed", "unknown", "pending":
			_, err = tx.Exec(ctx, `
				UPDATE transactions SET print_state = $2, updated_at = now()
				WHERE id = $1`, input.TransactionID, input.Status)
		}
		if err != nil {
			return domain.PrintAttempt{}, dbError(err, "update transaction print state")
		}
	}
	if err = audit(ctx, tx, "print_attempt.recorded", "transaction", input.TransactionID,
		input.Identity, nil, attempt, nil, input.OccurredAt); err != nil {
		return domain.PrintAttempt{}, dbError(err, "audit print attempt")
	}
	if err = addChange(ctx, tx, "print_attempt", attempt.ID.String(), "created",
		&input.Revision, attempt, false); err != nil {
		return domain.PrintAttempt{}, dbError(err, "sync print attempt")
	}
	return attempt, nil
}
