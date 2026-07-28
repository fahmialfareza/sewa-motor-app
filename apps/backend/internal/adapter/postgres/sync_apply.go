package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ApplySyncMutation performs the business mutation, audit/change writes, and
// idempotency result write in one PostgreSQL transaction. A transaction-scoped
// advisory lock serializes retries for the same terminal operation ID.
func (s *Store) ApplySyncMutation(
	ctx context.Context,
	submitter domain.Principal,
	operation domain.SyncMutation,
	requestHash []byte,
) (domain.StoredOperationResult, bool, error) {
	defer observability.StartSegment(ctx, "Postgres.ApplySyncMutation")()
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return domain.StoredOperationResult{}, false, dbError(err, "begin sync operation")
	}
	defer tx.Rollback(ctx)
	lockKey := operation.TerminalID.String() + ":" + operation.OperationID
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return domain.StoredOperationResult{}, false, dbError(err, "lock sync operation")
	}

	var stored domain.StoredOperationResult
	err = tx.QueryRow(ctx, `
		SELECT request_hash, response_status, response
		FROM idempotency_records
		WHERE terminal_id = $1 AND operation_id = $2`,
		operation.TerminalID, operation.OperationID,
	).Scan(&stored.RequestHash, &stored.Status, &stored.Response)
	if err == nil {
		if !bytes.Equal(stored.RequestHash, requestHash) {
			return domain.StoredOperationResult{}, false,
				domain.NewError(domain.CodeIdempotencyMismatch, "operationId telah digunakan untuk payload yang berbeda")
		}
		if err = tx.Commit(ctx); err != nil {
			return domain.StoredOperationResult{}, false, dbError(err, "commit replayed sync operation")
		}
		if stored.Status >= 400 {
			var operationError domain.Error
			if json.Unmarshal(stored.Response, &operationError) == nil {
				return stored, true, &operationError
			}
		}
		return stored, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.StoredOperationResult{}, false, dbError(err, "read sync idempotency")
	}
	if _, err = tx.Exec(ctx, `SAVEPOINT sync_business_mutation`); err != nil {
		return domain.StoredOperationResult{}, false, dbError(err, "savepoint sync mutation")
	}

	identity := domain.MutationIdentity{
		OriginActorID:        operation.OriginActorID,
		OriginSessionID:      operation.OriginSessionID,
		TerminalID:           &operation.TerminalID,
		SubmittedByActorID:   submitter.UserID,
		SubmittedBySessionID: submitter.SessionID,
	}
	var data any
	status := http.StatusOK
	operationErr := error(nil)

	switch operation.Aggregate + "/" + operation.Action {
	case "transaction/create":
		var payload struct {
			ID    string             `json:"id"`
			Items []domain.ItemInput `json:"items"`
		}
		if err := json.Unmarshal(operation.Payload, &payload); err != nil || operation.OccurredAt.IsZero() {
			operationErr = domain.Validation("Payload transaksi tidak valid", nil)
			break
		}
		if operation.AggregateID != "" && operation.AggregateID != payload.ID {
			operationErr = domain.Validation("aggregateId tidak cocok dengan transaksi", nil)
			break
		}
		if err := domain.ValidateTransactionID(payload.ID); err != nil {
			operationErr = err
			break
		}
		if err := domain.ValidateItems(payload.Items); err != nil {
			operationErr = err
			break
		}
		data, operationErr = s.createTransactionTx(ctx, tx, domain.CreateTransactionInput{
			ID: payload.ID, OccurredAt: operation.OccurredAt, Items: payload.Items, Identity: identity,
		})
		status = http.StatusCreated
	case "transaction/correct":
		var payload struct {
			BaseRevision int                `json:"baseRevision"`
			Reason       string             `json:"reason"`
			Items        []domain.ItemInput `json:"items"`
		}
		if err := json.Unmarshal(operation.Payload, &payload); err != nil || operation.AggregateID == "" {
			operationErr = domain.Validation("Payload koreksi tidak valid", nil)
			break
		}
		baseRevision := payload.BaseRevision
		if operation.BaseRevision != nil {
			baseRevision = *operation.BaseRevision
		}
		if baseRevision < 1 || len(strings.TrimSpace(payload.Reason)) < 5 {
			operationErr = domain.Validation("Revisi dasar dan alasan koreksi wajib diisi", nil)
			break
		}
		if err := domain.ValidateItems(payload.Items); err != nil {
			operationErr = err
			break
		}
		data, operationErr = s.correctTransactionTx(ctx, tx, domain.CorrectTransactionInput{
			ID: operation.AggregateID, BaseRevision: baseRevision, Reason: strings.TrimSpace(payload.Reason),
			OccurredAt: operation.OccurredAt, Items: payload.Items, Identity: identity,
		})
	case "print_attempt/create":
		var payload domain.PrintAttemptInput
		if err := json.Unmarshal(operation.Payload, &payload); err != nil || payload.TransactionID == "" {
			operationErr = domain.Validation("Payload upaya cetak tidak valid", nil)
			break
		}
		payload.Identity = identity
		payload.OccurredAt = operation.OccurredAt
		if payload.ID == uuid.Nil || operation.AggregateID != payload.ID.String() || payload.Revision < 1 || payload.OccurredAt.IsZero() {
			operationErr = domain.Validation("ID, revisi, dan waktu cetak tidak valid", nil)
			break
		}
		data, operationErr = s.recordPrintAttemptTx(ctx, tx, payload)
		status = http.StatusCreated
	default:
		operationErr = domain.Validation("Jenis operasi sinkronisasi tidak didukung", nil)
	}

	if operationErr != nil {
		if _, rollbackErr := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT sync_business_mutation`); rollbackErr != nil {
			return domain.StoredOperationResult{}, false, dbError(rollbackErr, "rollback rejected sync mutation")
		}
		domainErr := domain.AsError(operationErr)
		status = operationErrorStatus(domainErr)
		if status >= 500 {
			return domain.StoredOperationResult{}, false, operationErr
		}
		stored = domain.StoredOperationResult{RequestHash: requestHash, Status: status}
		stored.Response, err = json.Marshal(domainErr)
		if err != nil {
			return domain.StoredOperationResult{}, false, domain.WrapInternal(err, "marshal sync error result")
		}
	} else {
		stored = domain.StoredOperationResult{RequestHash: requestHash, Status: status}
		stored.Response, err = json.Marshal(data)
		if err != nil {
			return domain.StoredOperationResult{}, false, domain.WrapInternal(err, "marshal sync success result")
		}
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO idempotency_records (
			terminal_id, operation_id, request_hash, response_status, response
		) VALUES ($1,$2,$3,$4,$5)`,
		operation.TerminalID, operation.OperationID, requestHash, stored.Status, stored.Response,
	); err != nil {
		return domain.StoredOperationResult{}, false, dbError(err, "persist atomic sync result")
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.StoredOperationResult{}, false, dbError(err, "commit atomic sync result")
	}
	return stored, false, operationErr
}

func operationErrorStatus(err *domain.Error) int {
	switch err.Code {
	case domain.CodeValidation:
		return http.StatusUnprocessableEntity
	case domain.CodeUnauthorized:
		return http.StatusUnauthorized
	case domain.CodeForbidden, domain.CodePasswordChange, domain.CodeSignatureInvalid:
		return http.StatusForbidden
	case domain.CodeNotFound:
		return http.StatusNotFound
	case domain.CodeRevisionConflict, domain.CodeConflict, domain.CodeIdempotencyMismatch:
		return http.StatusConflict
	case domain.CodeRateLimited:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}
