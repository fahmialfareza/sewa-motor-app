package postgres

import (
	"context"
	"encoding/json"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Store) SetTransactionPaymentStatus(
	ctx context.Context,
	input domain.SetPaymentStatusInput,
) (domain.Transaction, error) {
	defer observability.StartSegment(ctx, "Postgres.SetTransactionPaymentStatus")()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.Transaction{}, dbError(err, "begin payment status")
	}
	defer tx.Rollback(ctx)

	transaction, err := s.setTransactionPaymentStatusTx(ctx, tx, input)
	if err != nil {
		return domain.Transaction{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Transaction{}, dbError(err, "commit payment status")
	}
	return transaction, nil
}

func (s *Store) setTransactionPaymentStatusTx(
	ctx context.Context,
	tx pgx.Tx,
	input domain.SetPaymentStatusInput,
) (domain.Transaction, error) {
	defer observability.StartSegment(ctx, "Postgres.setTransactionPaymentStatusTx")()

	if err := domain.ValidatePaymentOutcome(input.Status); err != nil {
		return domain.Transaction{}, err
	}

	var currentRevision int
	var currentMethod domain.PaymentMethod
	var currentQrisPayloadHash *string
	var currentStatus domain.PaymentStatus
	var currentConfirmedRevision *int
	var currentSnapshot json.RawMessage
	var deletedAt any
	var ownerID uuid.UUID
	var actingRole domain.Role
	err := tx.QueryRow(ctx, `
		SELECT t.current_revision, t.payment_method, t.payment_status,
		       t.qris_payload_hash, t.payment_confirmed_revision,
		       r.after_snapshot, t.deleted_at,
		       t.origin_actor_id, acting_user.role
		FROM transactions t
		JOIN transaction_revisions r
		  ON r.transaction_id = t.id AND r.revision = t.current_revision
		JOIN users acting_user ON acting_user.id = $2
		WHERE t.id = $1
		FOR UPDATE OF t`,
		input.ID, input.Identity.OriginActorID,
	).Scan(
		&currentRevision, &currentMethod, &currentStatus, &currentQrisPayloadHash,
		&currentConfirmedRevision, &currentSnapshot, &deletedAt,
		&ownerID, &actingRole,
	)
	if err != nil {
		return domain.Transaction{}, dbError(err, "lock transaction payment")
	}
	if !domain.CanCorrectTransaction(actingRole, input.Identity.OriginActorID, ownerID) {
		return domain.Transaction{}, domain.NewError(
			domain.CodeForbidden,
			"Admin hanya dapat mengubah pembayaran transaksi miliknya sendiri",
		)
	}
	if deletedAt != nil {
		return domain.Transaction{}, domain.NewError(
			domain.CodeConflict,
			"Pembayaran transaksi yang dihapus tidak dapat diubah",
		)
	}
	if currentRevision != input.BaseRevision {
		serverSnapshot := paymentStateSnapshotJSON(
			domain.NormalizeTransactionSnapshot(currentSnapshot, currentRevision),
			currentStatus,
			currentConfirmedRevision,
		)
		serverSnapshot["paymentMethod"] = currentMethod
		applyQrisPayloadHash(serverSnapshot, currentQrisPayloadHash)
		return domain.Transaction{}, &domain.Error{
			Code:    domain.CodePaymentStateConflict,
			Message: "Transaksi telah berubah di server",
			Details: map[string]any{
				"kind":                     "payment_state",
				"reason":                   "revision_changed",
				"baseRevision":             input.BaseRevision,
				"currentRevision":          currentRevision,
				"requestedStatus":          input.Status,
				"paymentStatus":            currentStatus,
				"paymentConfirmedRevision": currentConfirmedRevision,
				"serverSnapshot":           serverSnapshot,
			},
		}
	}
	confirmedRevision, changed, transitionErr := resolvePaymentTransition(
		currentStatus,
		input.Status,
		currentRevision,
	)
	if transitionErr != nil {
		return domain.Transaction{}, attachPaymentConflictServerSnapshot(
			transitionErr,
			currentSnapshot,
			currentRevision,
			currentMethod,
			currentQrisPayloadHash,
			currentStatus,
			currentConfirmedRevision,
		)
	}
	if !changed {
		transaction, readErr := getTransactionWith(ctx, tx, input.ID, true)
		if readErr != nil {
			return domain.Transaction{}, dbError(readErr, "read unchanged transaction payment")
		}
		return transaction, nil
	}

	eventType := "transaction.payment_failed"
	if input.Status == domain.PaymentStatusSuccess {
		eventType = "transaction.payment_succeeded"
	}
	if _, err = tx.Exec(ctx, `
		UPDATE transactions
		SET payment_status = $2, payment_confirmed_revision = $3,
		    updated_by = $4, updated_at = now()
		WHERE id = $1`,
		input.ID, input.Status, confirmedRevision, input.Identity.SubmittedByActorID,
	); err != nil {
		return domain.Transaction{}, dbError(err, "update transaction payment")
	}

	before := map[string]any{
		"revision":                 currentRevision,
		"paymentMethod":            currentMethod,
		"paymentStatus":            currentStatus,
		"paymentConfirmedRevision": currentConfirmedRevision,
	}
	applyQrisPayloadHash(before, currentQrisPayloadHash)
	after := map[string]any{
		"revision":                 currentRevision,
		"paymentMethod":            currentMethod,
		"paymentStatus":            input.Status,
		"paymentConfirmedRevision": confirmedRevision,
	}
	applyQrisPayloadHash(after, currentQrisPayloadHash)
	if err = audit(
		ctx,
		tx,
		eventType,
		"transaction",
		input.ID,
		input.Identity,
		before,
		after,
		nil,
		input.OccurredAt,
	); err != nil {
		return domain.Transaction{}, dbError(err, "audit transaction payment")
	}
	if err = addChange(
		ctx,
		tx,
		"transaction",
		input.ID,
		"updated",
		&currentRevision,
		after,
		false,
	); err != nil {
		return domain.Transaction{}, dbError(err, "sync transaction payment")
	}

	transaction, err := getTransactionWith(ctx, tx, input.ID, true)
	if err != nil {
		return domain.Transaction{}, dbError(err, "read updated transaction payment")
	}
	return transaction, nil
}

func attachPaymentConflictServerSnapshot(
	err error,
	currentSnapshot json.RawMessage,
	currentRevision int,
	currentMethod domain.PaymentMethod,
	currentQrisPayloadHash *string,
	currentStatus domain.PaymentStatus,
	currentConfirmedRevision *int,
) error {
	if !domain.IsCode(err, domain.CodePaymentStateConflict) {
		return err
	}
	domainErr := domain.AsError(err)
	if domainErr.Details == nil {
		domainErr.Details = make(map[string]any)
	}
	serverSnapshot := paymentStateSnapshotJSON(
		domain.NormalizeTransactionSnapshot(currentSnapshot, currentRevision),
		currentStatus,
		currentConfirmedRevision,
	)
	serverSnapshot["paymentMethod"] = currentMethod
	applyQrisPayloadHash(serverSnapshot, currentQrisPayloadHash)
	domainErr.Details["serverSnapshot"] = serverSnapshot
	return err
}

func resolvePaymentTransition(
	current domain.PaymentStatus,
	target domain.PaymentStatus,
	currentRevision int,
) (*int, bool, error) {
	if current == target {
		return nil, false, nil
	}
	if current == domain.PaymentStatusSuccess {
		confirmedRevision := currentRevision
		return nil, false, &domain.Error{
			Code:    domain.CodePaymentStateConflict,
			Message: "Pembayaran berhasil bersifat final untuk revisi transaksi ini",
			Details: map[string]any{
				"kind":                     "payment_state",
				"reason":                   "payment_final",
				"baseRevision":             currentRevision,
				"currentRevision":          currentRevision,
				"requestedStatus":          target,
				"paymentStatus":            current,
				"paymentConfirmedRevision": &confirmedRevision,
			},
		}
	}
	if target == domain.PaymentStatusSuccess {
		confirmedRevision := currentRevision
		return &confirmedRevision, true, nil
	}
	return nil, true, nil
}
