package postgres

import (
	"context"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
)

func (s *Store) ListTransactionRevisions(ctx context.Context, id string) ([]domain.TransactionRevision, error) {
	defer observability.StartSegment(ctx, "Postgres.ListTransactionRevisions")()
	if _, err := s.GetTransaction(ctx, id, true); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT transaction_id, revision, base_revision, change_type, reason,
		       before_snapshot, after_snapshot, origin_actor_id, submitted_by_actor_id,
		       terminal_id, client_occurred_at, server_received_at
		FROM transaction_revisions
		WHERE transaction_id = $1 AND revision > 1
		ORDER BY revision`,
		id,
	)
	if err != nil {
		return nil, dbError(err, "list transaction revisions")
	}
	defer rows.Close()
	result := make([]domain.TransactionRevision, 0)
	for rows.Next() {
		var revision domain.TransactionRevision
		if err := rows.Scan(
			&revision.TransactionID, &revision.Revision, &revision.BaseRevision,
			&revision.ChangeType, &revision.Reason, &revision.BeforeSnapshot,
			&revision.AfterSnapshot, &revision.OriginActorID, &revision.SubmittedBy,
			&revision.TerminalID, &revision.ClientOccurredAt, &revision.ServerReceivedAt,
		); err != nil {
			return nil, dbError(err, "scan transaction revision")
		}
		itemRows, err := s.Pool.Query(ctx, `
			SELECT line_number, package_id, package_revision, package_code, package_name,
			       package_description, unit_price, quantity, line_total
			FROM transaction_items
			WHERE transaction_id = $1 AND revision = $2
			ORDER BY line_number`,
			id, revision.Revision,
		)
		if err != nil {
			return nil, dbError(err, "list revision items")
		}
		revision.Items = make([]domain.TransactionItem, 0)
		for itemRows.Next() {
			var item domain.TransactionItem
			if err := itemRows.Scan(
				&item.LineNumber, &item.PackageID, &item.PackageRevision, &item.PackageCode,
				&item.PackageName, &item.PackageDescription, &item.UnitPrice,
				&item.Quantity, &item.LineTotal,
			); err != nil {
				itemRows.Close()
				return nil, dbError(err, "scan revision item")
			}
			revision.Items = append(revision.Items, item)
		}
		if err := itemRows.Err(); err != nil {
			itemRows.Close()
			return nil, dbError(err, "iterate revision items")
		}
		itemRows.Close()
		result = append(result, revision)
	}
	return result, dbError(rows.Err(), "iterate transaction revisions")
}

func (s *Store) ListPrintAttempts(ctx context.Context, id string) ([]domain.PrintAttempt, error) {
	defer observability.StartSegment(ctx, "Postgres.ListPrintAttempts")()
	if _, err := s.GetTransaction(ctx, id, true); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT id, transaction_id, transaction_revision, terminal_id, status, is_copy, actor_id,
		       printer_kind, printer_identifier, error_code, error_message, metadata,
		       client_occurred_at, server_received_at
		FROM print_attempts
		WHERE transaction_id = $1
		ORDER BY server_received_at, id`,
		id,
	)
	if err != nil {
		return nil, dbError(err, "list print attempts")
	}
	defer rows.Close()
	result := make([]domain.PrintAttempt, 0)
	for rows.Next() {
		var attempt domain.PrintAttempt
		if err := rows.Scan(
			&attempt.ID, &attempt.TransactionID, &attempt.TransactionRevision,
			&attempt.TerminalID, &attempt.Status, &attempt.IsCopy, &attempt.ActorID,
			&attempt.PrinterKind, &attempt.PrinterIdentifier, &attempt.ErrorCode,
			&attempt.ErrorMessage, &attempt.Metadata, &attempt.ClientOccurredAt,
			&attempt.ServerReceivedAt,
		); err != nil {
			return nil, dbError(err, "scan print attempt")
		}
		result = append(result, attempt)
	}
	return result, dbError(rows.Err(), "iterate print attempts")
}
