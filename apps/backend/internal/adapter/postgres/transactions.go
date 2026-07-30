package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type queryer interface {
	rowQuerier
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func (s *Store) CreateTransaction(ctx context.Context, input domain.CreateTransactionInput) (domain.Transaction, error) {
	defer observability.StartSegment(ctx, "Postgres.CreateTransaction")()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.Transaction{}, dbError(err, "begin transaction create")
	}
	defer tx.Rollback(ctx)
	transaction, err := s.createTransactionTx(ctx, tx, input)
	if err != nil {
		return domain.Transaction{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Transaction{}, dbError(err, "commit transaction create")
	}
	return transaction, nil
}

func (s *Store) createTransactionTx(ctx context.Context, tx pgx.Tx, input domain.CreateTransactionInput) (domain.Transaction, error) {
	defer observability.StartSegment(ctx, "Postgres.createTransactionTx")()
	paymentStatus := input.InitialPaymentStatus
	if paymentStatus == "" {
		paymentStatus = domain.PaymentStatusPending
	}
	if !input.PaymentMethod.Valid() || !paymentStatus.Valid() {
		return domain.Transaction{}, domain.Validation("Data pembayaran transaksi tidak valid", nil)
	}
	if err := domain.ValidateQrisPayloadBinding(input.PaymentMethod, input.QrisPayloadHash); err != nil {
		return domain.Transaction{}, err
	}
	confirmedRevision := input.InitialPaymentConfirmedRevision
	switch paymentStatus {
	case domain.PaymentStatusSuccess:
		if confirmedRevision == nil || *confirmedRevision != 1 {
			return domain.Transaction{}, domain.Validation("Revisi konfirmasi pembayaran tidak valid", nil)
		}
	case domain.PaymentStatusPending, domain.PaymentStatusFailed:
		if confirmedRevision != nil {
			return domain.Transaction{}, domain.Validation("Pembayaran belum berhasil tidak boleh memiliki revisi konfirmasi", nil)
		}
	}
	items, total, err := resolveTransactionItems(
		ctx,
		tx,
		input.PaymentMethod,
		input.Items,
	)
	if err != nil {
		return domain.Transaction{}, err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO transactions (
			id, current_revision, occurred_at,
			origin_actor_id, origin_session_id, terminal_id, updated_by,
			subtotal, total, payment_method, qris_payload_hash,
			payment_status, payment_confirmed_revision
		) VALUES ($1,1,$2,$3,$4,$5,$6,$7,$7,$8,$9,$10,$11)`,
		input.ID, input.OccurredAt, input.Identity.OriginActorID,
		input.Identity.OriginSessionID, input.Identity.TerminalID,
		input.Identity.SubmittedByActorID, total, input.PaymentMethod,
		input.QrisPayloadHash, paymentStatus, confirmedRevision,
	)
	if err != nil {
		return domain.Transaction{}, dbError(err, "insert transaction")
	}
	after := transactionSnapshot(
		input.OccurredAt,
		input.PaymentMethod,
		input.QrisPayloadHash,
		items,
		total,
	)
	createdState := paymentStateSnapshot(after, paymentStatus, confirmedRevision)
	afterJSON, err := json.Marshal(createdState)
	if err != nil {
		return domain.Transaction{}, domain.WrapInternal(err, "marshal transaction snapshot")
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO transaction_revisions (
			transaction_id, revision, change_type, qris_payload_hash, after_snapshot,
			origin_actor_id, origin_session_id, terminal_id,
			submitted_by_actor_id, submitted_by_session_id, client_occurred_at
		) VALUES ($1,1,'create',$2,$3,$4,$5,$6,$7,$8,$9)`,
		input.ID, input.QrisPayloadHash, afterJSON,
		input.Identity.OriginActorID, input.Identity.OriginSessionID, input.Identity.TerminalID,
		input.Identity.SubmittedByActorID, input.Identity.SubmittedBySessionID, input.OccurredAt,
	)
	if err != nil {
		return domain.Transaction{}, dbError(err, "insert transaction revision")
	}
	if err = insertItems(ctx, tx, input.ID, 1, items); err != nil {
		return domain.Transaction{}, err
	}
	if err = audit(ctx, tx, "transaction.created", "transaction", input.ID, input.Identity, nil, createdState, nil, input.OccurredAt); err != nil {
		return domain.Transaction{}, dbError(err, "audit transaction create")
	}
	revision := 1
	if err = addChange(ctx, tx, "transaction", input.ID, "created", &revision, createdState, false); err != nil {
		return domain.Transaction{}, dbError(err, "sync transaction create")
	}
	transaction, err := getTransactionWith(ctx, tx, input.ID, true)
	if err != nil {
		return domain.Transaction{}, dbError(err, "read created transaction")
	}
	return transaction, nil
}

func (s *Store) CorrectTransaction(ctx context.Context, input domain.CorrectTransactionInput) (domain.Transaction, error) {
	defer observability.StartSegment(ctx, "Postgres.CorrectTransaction")()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return domain.Transaction{}, dbError(err, "begin transaction correction")
	}
	defer tx.Rollback(ctx)
	transaction, err := s.correctTransactionTx(ctx, tx, input)
	if err != nil {
		return domain.Transaction{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Transaction{}, dbError(err, "commit transaction correction")
	}
	return transaction, nil
}

func (s *Store) correctTransactionTx(ctx context.Context, tx pgx.Tx, input domain.CorrectTransactionInput) (domain.Transaction, error) {
	defer observability.StartSegment(ctx, "Postgres.correctTransactionTx")()
	if input.LegacyPaymentCompatibility {
		if input.PaymentMethod != domain.PaymentMethodLegacy {
			return domain.Transaction{}, domain.Validation("Metode pembayaran legacy tidak valid", nil)
		}
	} else {
		if err := domain.ValidateSelectablePaymentMethod(input.PaymentMethod); err != nil {
			return domain.Transaction{}, err
		}
	}
	if err := domain.ValidateQrisPayloadBinding(input.PaymentMethod, input.QrisPayloadHash); err != nil {
		return domain.Transaction{}, err
	}
	var currentRevision int
	var transactionOccurredAt time.Time
	var beforeJSON []byte
	var latestPrintedRevision *int
	var currentPaymentMethod domain.PaymentMethod
	var currentQrisPayloadHash *string
	var paymentStatus domain.PaymentStatus
	var paymentConfirmedRevision *int
	var deletedAt any
	var ownerID uuid.UUID
	var correctingActorRole domain.Role
	err := tx.QueryRow(ctx, `
		SELECT t.current_revision, t.occurred_at, r.after_snapshot,
		       t.latest_printed_revision, t.payment_method,
		       t.qris_payload_hash, t.payment_status, t.payment_confirmed_revision,
		       t.deleted_at,
		       t.origin_actor_id, correcting_actor.role
		FROM transactions t
		JOIN transaction_revisions r
		  ON r.transaction_id = t.id AND r.revision = t.current_revision
		JOIN users correcting_actor ON correcting_actor.id = $2
		WHERE t.id = $1
		FOR UPDATE OF t`,
		input.ID, input.Identity.OriginActorID,
	).Scan(
		&currentRevision, &transactionOccurredAt, &beforeJSON, &latestPrintedRevision,
		&currentPaymentMethod, &currentQrisPayloadHash, &paymentStatus, &paymentConfirmedRevision,
		&deletedAt, &ownerID, &correctingActorRole,
	)
	if err != nil {
		return domain.Transaction{}, dbError(err, "lock corrected transaction")
	}
	beforeJSON = domain.NormalizeTransactionSnapshot(beforeJSON, currentRevision)
	beforeState := paymentStateSnapshotJSON(
		beforeJSON,
		paymentStatus,
		paymentConfirmedRevision,
	)
	applyQrisPayloadHash(beforeState, currentQrisPayloadHash)
	beforeJSON, err = json.Marshal(beforeState)
	if err != nil {
		return domain.Transaction{}, domain.WrapInternal(err, "marshal current transaction snapshot")
	}
	if !domain.CanCorrectTransaction(correctingActorRole, input.Identity.OriginActorID, ownerID) {
		return domain.Transaction{}, domain.NewError(
			domain.CodeForbidden,
			"Admin hanya dapat mengoreksi transaksi miliknya sendiri",
		)
	}
	if deletedAt != nil {
		return domain.Transaction{}, domain.NewError(domain.CodeConflict, "Transaksi yang dihapus tidak dapat dikoreksi")
	}
	if input.LegacyPaymentCompatibility && currentPaymentMethod != domain.PaymentMethodLegacy {
		return domain.Transaction{}, domain.NewError(
			domain.CodeConflict,
			"Koreksi lama hanya dapat diterapkan pada transaksi legacy",
		)
	}
	items, total, err := resolveTransactionItems(
		ctx,
		tx,
		input.PaymentMethod,
		input.Items,
	)
	if err != nil {
		return domain.Transaction{}, err
	}
	if currentRevision != input.BaseRevision {
		localStatus, localConfirmedRevision, _ := correctedPaymentState(
			input.LegacyPaymentCompatibility,
			input.BaseRevision+1,
		)
		local := paymentStateSnapshot(
			transactionSnapshot(
				transactionOccurredAt,
				input.PaymentMethod,
				input.QrisPayloadHash,
				items,
				total,
			),
			localStatus,
			localConfirmedRevision,
		)
		return domain.Transaction{}, &domain.Error{
			Code:    domain.CodeRevisionConflict,
			Message: "Transaksi telah berubah di server",
			Details: map[string]any{
				"baseRevision":    input.BaseRevision,
				"currentRevision": currentRevision,
				"localSnapshot":   local,
				"serverSnapshot":  json.RawMessage(beforeJSON),
			},
		}
	}
	nextRevision := currentRevision + 1
	nextPaymentStatus, nextPaymentConfirmedRevision, paymentReset := correctedPaymentState(
		input.LegacyPaymentCompatibility,
		nextRevision,
	)
	after := transactionSnapshot(
		transactionOccurredAt,
		input.PaymentMethod,
		input.QrisPayloadHash,
		items,
		total,
	)
	afterState := paymentStateSnapshot(
		after,
		nextPaymentStatus,
		nextPaymentConfirmedRevision,
	)
	afterJSON, err := json.Marshal(afterState)
	if err != nil {
		return domain.Transaction{}, domain.WrapInternal(err, "marshal corrected snapshot")
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO transaction_revisions (
			transaction_id, revision, base_revision, change_type, reason,
			qris_payload_hash, before_snapshot, after_snapshot,
			origin_actor_id, origin_session_id, terminal_id,
			submitted_by_actor_id, submitted_by_session_id, client_occurred_at
		) VALUES ($1,$2,$3,'correction',$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		input.ID, nextRevision, currentRevision, input.Reason,
		input.QrisPayloadHash, beforeJSON, afterJSON,
		input.Identity.OriginActorID, input.Identity.OriginSessionID, input.Identity.TerminalID,
		input.Identity.SubmittedByActorID, input.Identity.SubmittedBySessionID, input.OccurredAt,
	)
	if err != nil {
		return domain.Transaction{}, dbError(err, "insert transaction correction")
	}
	if err = insertItems(ctx, tx, input.ID, nextRevision, items); err != nil {
		return domain.Transaction{}, err
	}
	printState := "pending"
	if latestPrintedRevision != nil {
		printState = "needs-reprint"
	}
	_, err = tx.Exec(ctx, `
		UPDATE transactions
		SET current_revision = $2, subtotal = $3, total = $3,
		    updated_by = $4, payment_method = $5,
		    qris_payload_hash = $6, payment_status = $7,
		    payment_confirmed_revision = $8,
		    updated_at = now(), print_state = $9
		WHERE id = $1`,
		input.ID, nextRevision, total, input.Identity.SubmittedByActorID,
		input.PaymentMethod, input.QrisPayloadHash, nextPaymentStatus,
		nextPaymentConfirmedRevision, printState,
	)
	if err != nil {
		return domain.Transaction{}, dbError(err, "advance transaction correction")
	}
	if err = audit(ctx, tx, "transaction.corrected", "transaction", input.ID, input.Identity,
		beforeState,
		afterState,
		map[string]any{
			"reason": input.Reason, "paymentReset": paymentReset,
			"legacyPaymentCompatibility": input.LegacyPaymentCompatibility,
		}, input.OccurredAt); err != nil {
		return domain.Transaction{}, dbError(err, "audit transaction correction")
	}
	if err = addChange(
		ctx,
		tx,
		"transaction",
		input.ID,
		"updated",
		&nextRevision,
		afterState,
		false,
	); err != nil {
		return domain.Transaction{}, dbError(err, "sync transaction correction")
	}
	transaction, err := getTransactionWith(ctx, tx, input.ID, true)
	if err != nil {
		return domain.Transaction{}, dbError(err, "read corrected transaction")
	}
	return transaction, nil
}

func resolveItems(ctx context.Context, query rowQuerier, inputs []domain.ItemInput) ([]domain.TransactionItem, int64, error) {
	defer observability.StartSegment(ctx, "Postgres.resolveItems")()
	items := make([]domain.TransactionItem, 0, len(inputs))
	var total int64
	for index, input := range inputs {
		var snapshot domain.PackageSnapshot
		err := query.QueryRow(ctx, `
			SELECT p.id, r.revision, p.code, r.name, r.description, r.unit_price
			FROM packages p
			JOIN package_revisions r ON r.package_id = p.id
			WHERE p.id = $1 AND r.revision = $2`,
			input.PackageID, input.PackageRevision,
		).Scan(
			&snapshot.ID, &snapshot.Revision, &snapshot.Code, &snapshot.Name,
			&snapshot.Description, &snapshot.UnitPrice,
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, 0, &domain.Error{
					Code: domain.CodeValidation, Message: "Revisi paket tidak ditemukan",
					Details: map[string]any{"index": index, "packageId": input.PackageID, "revision": input.PackageRevision},
				}
			}
			return nil, 0, dbError(err, "resolve package snapshot")
		}
		lineTotal, err := domain.CheckedLineTotal(snapshot.UnitPrice, input.Quantity)
		if err != nil {
			return nil, 0, err
		}
		if total > math.MaxInt64-lineTotal {
			return nil, 0, domain.Validation("Total transaksi terlalu besar", nil)
		}
		total += lineTotal
		items = append(items, domain.TransactionItem{
			LineNumber: index + 1, PackageID: snapshot.ID, PackageRevision: snapshot.Revision,
			PackageCode: snapshot.Code, PackageName: snapshot.Name, PackageDescription: snapshot.Description,
			UnitPrice: snapshot.UnitPrice, Quantity: input.Quantity, LineTotal: lineTotal,
		})
	}
	return items, total, nil
}

func resolveTransactionItems(
	ctx context.Context,
	query rowQuerier,
	paymentMethod domain.PaymentMethod,
	inputs []domain.ItemInput,
) ([]domain.TransactionItem, int64, error) {
	defer observability.StartSegment(ctx, "Postgres.resolveTransactionItems")()
	items, total, err := resolveItems(ctx, query, inputs)
	if err != nil {
		return nil, 0, err
	}
	if err := domain.ValidatePaymentTotal(paymentMethod, total); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func insertItems(ctx context.Context, tx pgx.Tx, transactionID string, revision int, items []domain.TransactionItem) error {
	defer observability.StartSegment(ctx, "Postgres.insertItems")()
	for _, item := range items {
		_, err := tx.Exec(ctx, `
			INSERT INTO transaction_items (
				transaction_id, revision, line_number,
				package_id, package_revision, package_code, package_name, package_description,
				unit_price, quantity, line_total
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			transactionID, revision, item.LineNumber,
			item.PackageID, item.PackageRevision, item.PackageCode, item.PackageName, item.PackageDescription,
			item.UnitPrice, item.Quantity, item.LineTotal,
		)
		if err != nil {
			return dbError(err, "insert transaction item")
		}
	}
	return nil
}

func transactionSnapshot(
	occurredAt any,
	paymentMethod domain.PaymentMethod,
	qrisPayloadHash *string,
	items []domain.TransactionItem,
	total int64,
) map[string]any {
	apiItems := make([]map[string]any, 0, len(items))
	for _, item := range items {
		apiItems = append(apiItems, map[string]any{
			"packageId": item.PackageID, "packageRevision": item.PackageRevision,
			"name": item.PackageName, "description": item.PackageDescription,
			"unitPrice": item.UnitPrice, "quantity": item.Quantity, "lineTotal": item.LineTotal,
		})
	}
	snapshot := map[string]any{
		"occurredAt": occurredAt, "paymentMethod": paymentMethod,
		"items": apiItems, "subtotal": total, "total": total,
	}
	applyQrisPayloadHash(snapshot, qrisPayloadHash)
	return snapshot
}

func paymentStateSnapshot(
	snapshot map[string]any,
	status domain.PaymentStatus,
	confirmedRevision *int,
) map[string]any {
	result := make(map[string]any, len(snapshot)+2)
	for key, value := range snapshot {
		result[key] = value
	}
	result["paymentStatus"] = status
	result["paymentConfirmedRevision"] = confirmedRevision
	return result
}

func paymentStateSnapshotJSON(
	snapshot json.RawMessage,
	status domain.PaymentStatus,
	confirmedRevision *int,
) map[string]any {
	result := make(map[string]any)
	if err := json.Unmarshal(snapshot, &result); err != nil {
		result = map[string]any{"snapshot": snapshot}
	}
	result["paymentStatus"] = status
	result["paymentConfirmedRevision"] = confirmedRevision
	return result
}

func applyQrisPayloadHash(snapshot map[string]any, hash *string) {
	if hash == nil {
		delete(snapshot, "qrisPayloadHash")
		return
	}
	snapshot["qrisPayloadHash"] = *hash
}

func correctedPaymentState(
	legacyCompatibility bool,
	revision int,
) (domain.PaymentStatus, *int, bool) {
	if legacyCompatibility {
		confirmedRevision := revision
		return domain.PaymentStatusSuccess, &confirmedRevision, false
	}
	return domain.PaymentStatusPending, nil, true
}

func (s *Store) GetTransaction(ctx context.Context, id string, includeDeleted bool) (domain.Transaction, error) {
	defer observability.StartSegment(ctx, "Postgres.GetTransaction")()
	transaction, err := getTransactionWith(ctx, s.Pool, id, includeDeleted)
	return transaction, dbError(err, "get transaction")
}

func getTransactionWith(ctx context.Context, query queryer, id string, includeDeleted bool) (domain.Transaction, error) {
	defer observability.StartSegment(ctx, "Postgres.getTransactionWith")()
	var item domain.Transaction
	var deletedActorID *uuid.UUID
	var deletedActorName, deletedActorUsername *string
	var deletedActorRole *domain.Role
	sql := `
		SELECT t.id, t.current_revision, t.occurred_at, t.server_received_at,
		       t.subtotal, t.total, t.payment_method, t.payment_status,
		       t.qris_payload_hash, t.payment_confirmed_revision,
		       t.print_state, t.latest_printed_revision,
		       t.terminal_id, t.deleted_at, t.delete_reason, t.updated_at,
		       origin.id, origin.full_name, origin.username, origin.role,
		       updater.id, updater.full_name, updater.username, updater.role,
		       deleter.id, deleter.full_name, deleter.username, deleter.role
		FROM transactions t
		JOIN users origin ON origin.id = t.origin_actor_id
		JOIN users updater ON updater.id = t.updated_by
		LEFT JOIN users deleter ON deleter.id = t.deleted_by
		WHERE t.id = $1`
	if !includeDeleted {
		sql += ` AND t.deleted_at IS NULL`
	}
	err := query.QueryRow(ctx, sql, id).Scan(
		&item.ID, &item.Revision, &item.OccurredAt, &item.ServerReceivedAt,
		&item.Subtotal, &item.Total, &item.PaymentMethod, &item.PaymentStatus,
		&item.QrisPayloadHash, &item.PaymentConfirmedRevision,
		&item.PrintState, &item.LatestPrintedRevision,
		&item.TerminalID, &item.DeletedAt, &item.DeleteReason, &item.UpdatedAt,
		&item.OriginActor.ID, &item.OriginActor.FullName, &item.OriginActor.Username, &item.OriginActor.Role,
		&item.UpdatedBy.ID, &item.UpdatedBy.FullName, &item.UpdatedBy.Username, &item.UpdatedBy.Role,
		&deletedActorID, &deletedActorName, &deletedActorUsername, &deletedActorRole,
	)
	if err != nil {
		return domain.Transaction{}, err
	}
	item.DisplayID = "TRX-" + item.ID
	if deletedActorID != nil {
		item.DeletedBy = &domain.ActorSummary{
			ID: *deletedActorID, FullName: stringValue(deletedActorName),
			Username: stringValue(deletedActorUsername), Role: roleValue(deletedActorRole),
		}
	}
	rows, err := query.Query(ctx, `
		SELECT line_number, package_id, package_revision, package_code, package_name,
		       package_description, unit_price, quantity, line_total
		FROM transaction_items
		WHERE transaction_id = $1 AND revision = $2
		ORDER BY line_number`,
		id, item.Revision,
	)
	if err != nil {
		return domain.Transaction{}, err
	}
	defer rows.Close()
	item.Items = make([]domain.TransactionItem, 0)
	for rows.Next() {
		var line domain.TransactionItem
		if err := rows.Scan(
			&line.LineNumber, &line.PackageID, &line.PackageRevision, &line.PackageCode,
			&line.PackageName, &line.PackageDescription, &line.UnitPrice, &line.Quantity, &line.LineTotal,
		); err != nil {
			return domain.Transaction{}, err
		}
		item.Items = append(item.Items, line)
	}
	return item, rows.Err()
}

func (s *Store) ListTransactions(ctx context.Context, filter domain.TransactionFilter) (domain.TransactionPage, error) {
	defer observability.StartSegment(ctx, "Postgres.ListTransactions")()
	args := make([]any, 0, 8)
	conditions := []string{"1=1"}
	add := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}
	if !filter.IncludeDeleted {
		conditions = append(conditions, "t.deleted_at IS NULL")
	}
	if search := strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(filter.Search)), "TRX-"); search != "" {
		conditions = append(conditions, "t.id LIKE "+add(search+"%"))
	}
	if filter.From != nil {
		conditions = append(conditions, "t.occurred_at >= "+add(*filter.From))
	}
	if filter.To != nil {
		conditions = append(conditions, "t.occurred_at < "+add(*filter.To))
	}
	if filter.CreatorID != nil {
		conditions = append(conditions, "t.origin_actor_id = "+add(*filter.CreatorID))
	}
	if filter.PackageID != nil {
		placeholder := add(*filter.PackageID)
		conditions = append(conditions, `EXISTS (
			SELECT 1 FROM transaction_items fi
			WHERE fi.transaction_id = t.id AND fi.revision = t.current_revision
			  AND fi.package_id = `+placeholder+`)`)
	}
	if filter.PaymentMethod != nil {
		conditions = append(conditions, "t.payment_method = "+add(*filter.PaymentMethod))
	}
	if filter.PaymentStatus != nil {
		conditions = append(conditions, "t.payment_status = "+add(*filter.PaymentStatus))
	}
	if filter.CursorOccurred != nil && filter.CursorID != "" {
		at := add(*filter.CursorOccurred)
		id := add(filter.CursorID)
		conditions = append(conditions, "(t.occurred_at, t.id) < ("+at+","+id+")")
	}
	limit := filter.Limit
	if limit < 1 || limit > 100 {
		limit = 25
	}
	args = append(args, limit+1)
	sql := `
		SELECT t.id
		FROM transactions t
		WHERE ` + strings.Join(conditions, " AND ") + `
		ORDER BY t.occurred_at DESC, t.id DESC
		LIMIT $` + fmt.Sprintf("%d", len(args))
	rows, err := s.Pool.Query(ctx, sql, args...)
	if err != nil {
		return domain.TransactionPage{}, dbError(err, "list transaction ids")
	}
	ids := make([]string, 0, limit+1)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return domain.TransactionPage{}, dbError(err, "scan transaction id")
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return domain.TransactionPage{}, dbError(err, "iterate transaction ids")
	}
	rows.Close()
	hasMore := len(ids) > limit
	if hasMore {
		ids = ids[:limit]
	}
	page := domain.TransactionPage{Transactions: make([]domain.Transaction, 0, len(ids))}
	for _, id := range ids {
		item, err := getTransactionWith(ctx, s.Pool, id, filter.IncludeDeleted)
		if err != nil {
			return domain.TransactionPage{}, dbError(err, "hydrate transaction list")
		}
		page.Transactions = append(page.Transactions, item)
	}
	if hasMore && len(page.Transactions) > 0 {
		last := page.Transactions[len(page.Transactions)-1]
		page.NextCursor = domain.EncodeCursor(domain.Cursor{OccurredAt: last.OccurredAt, ID: last.ID})
	}
	return page, nil
}

func (s *Store) DeleteTransaction(ctx context.Context, actor domain.Principal, id, reason string) error {
	defer observability.StartSegment(ctx, "Postgres.DeleteTransaction")()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return dbError(err, "begin delete transaction")
	}
	defer tx.Rollback(ctx)
	before, err := getTransactionWith(ctx, tx, id, true)
	if err != nil {
		return dbError(err, "get deleted transaction")
	}
	if before.DeletedAt != nil {
		return domain.NewError(domain.CodeConflict, "Transaksi sudah dihapus")
	}
	_, err = tx.Exec(ctx, `
		UPDATE transactions
		SET deleted_at = now(), deleted_by = $2, delete_reason = $3, updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL`,
		id, actor.UserID, reason,
	)
	if err != nil {
		return dbError(err, "delete transaction")
	}
	identity := principalIdentity(actor)
	after := map[string]any{"id": id, "revision": before.Revision, "deleted": true, "reason": reason}
	if err = audit(ctx, tx, "transaction.deleted", "transaction", id, identity, before, after,
		map[string]any{"reason": reason}, s.Now()); err != nil {
		return dbError(err, "audit delete transaction")
	}
	if err = addChange(ctx, tx, "transaction", id, "deleted", &before.Revision, after, true); err != nil {
		return dbError(err, "sync delete transaction")
	}
	return dbError(tx.Commit(ctx), "commit delete transaction")
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func roleValue(value *domain.Role) domain.Role {
	if value == nil {
		return ""
	}
	return *value
}
