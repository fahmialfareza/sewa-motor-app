package usecase

import (
	"context"
	"crypto/sha256"
	"net/http"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/adapter/security"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/port"
)

type Sync struct {
	Repo         port.Repository
	Transactions Transactions
}

func (s Sync) Push(ctx context.Context, principal domain.Principal, operations []domain.SyncMutation) ([]domain.SyncOperationResult, error) {
	defer observability.StartSegment(ctx, "Usecase.Sync.Push")()
	if err := RequireReady(principal); err != nil {
		return nil, err
	}
	if len(operations) == 0 || len(operations) > 100 {
		return nil, domain.Validation("Batch sinkronisasi harus berisi 1–100 operasi", nil)
	}
	results := make([]domain.SyncOperationResult, 0, len(operations))
	for _, operation := range operations {
		results = append(results, s.apply(ctx, principal, operation))
	}
	return results, nil
}

func (s Sync) Pull(ctx context.Context, principal domain.Principal, cursor int64, limit int) ([]domain.SyncChange, int64, bool, error) {
	defer observability.StartSegment(ctx, "Usecase.Sync.Pull")()
	if err := RequireReady(principal); err != nil {
		return nil, cursor, false, err
	}
	if cursor < 0 {
		return nil, cursor, false, domain.Validation("Cursor sinkronisasi tidak valid", nil)
	}
	if limit < 1 || limit > 500 {
		limit = 200
	}
	changes, err := s.Repo.PullChanges(ctx, cursor, limit+1)
	if err != nil {
		return nil, cursor, false, err
	}
	hasMore := len(changes) > limit
	if hasMore {
		changes = changes[:limit]
	}
	next := cursor
	if len(changes) > 0 {
		next = changes[len(changes)-1].Cursor
	}
	return changes, next, hasMore, nil
}

func (s Sync) apply(ctx context.Context, principal domain.Principal, operation domain.SyncMutation) domain.SyncOperationResult {
	defer observability.StartSegment(ctx, "Usecase.Sync.Apply")()
	result := domain.SyncOperationResult{OperationID: operation.OperationID}
	canonical, err := security.CanonicalMutation(operation)
	if err != nil {
		return syncError(ctx, result, domain.Validation("Payload operasi tidak dapat dikanonisasi", nil))
	}
	hash := sha256.Sum256(canonical)
	if principal.TerminalID == nil || *principal.TerminalID != operation.TerminalID {
		return syncError(ctx, result, domain.NewError(domain.CodeForbidden, "Sesi pengirim tidak terikat ke terminal operasi"))
	}
	publicKey, err := s.Repo.TerminalPublicKey(ctx, operation.TerminalID)
	if err != nil {
		return syncError(ctx, result, err)
	}
	if err := security.VerifyMutation(publicKey, operation); err != nil {
		return syncError(ctx, result, domain.NewError(domain.CodeSignatureInvalid, "Tanda tangan terminal tidak valid"))
	}
	matches, err := s.Repo.OriginSessionMatches(ctx, operation.OriginSessionID, operation.OriginActorID, operation.TerminalID)
	if err != nil {
		return syncError(ctx, result, err)
	}
	if !matches {
		return syncError(ctx, result, domain.NewError(domain.CodeForbidden, "Identitas asal operasi tidak cocok dengan terminal"))
	}

	stored, replayed, operationErr := s.Repo.ApplySyncMutation(ctx, principal, operation, hash[:])
	result.Status = stored.Status
	result.Replayed = replayed
	if operationErr != nil {
		result = syncError(ctx, result, operationErr)
		result.Replayed = replayed
		return result
	}
	result.Data = stored.Response
	return result
}

func syncError(ctx context.Context, result domain.SyncOperationResult, err error) domain.SyncOperationResult {
	defer observability.StartSegment(ctx, "Usecase.Sync.Error")()
	observability.NoticeError(ctx, err, "sync operation")
	domainErr := domain.AsError(err)
	result.Error = domainErr
	switch domainErr.Code {
	case domain.CodeValidation:
		result.Status = http.StatusUnprocessableEntity
	case domain.CodeUnauthorized:
		result.Status = http.StatusUnauthorized
	case domain.CodeForbidden, domain.CodePasswordChange:
		result.Status = http.StatusForbidden
	case domain.CodeNotFound:
		result.Status = http.StatusNotFound
	case domain.CodeRevisionConflict, domain.CodePaymentStateConflict,
		domain.CodeConflict, domain.CodeIdempotencyMismatch:
		result.Status = http.StatusConflict
	default:
		result.Status = http.StatusInternalServerError
	}
	return result
}
