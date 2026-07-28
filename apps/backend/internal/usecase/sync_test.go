package usecase

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/adapter/security"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/port"
	"github.com/google/uuid"
)

type syncRepository struct {
	port.Repository
	publicKey ed25519.PublicKey
	result    domain.StoredOperationResult
	replayed  bool
	applyErr  error
	matches   bool
}

func (r *syncRepository) TerminalPublicKey(context.Context, uuid.UUID) ([]byte, error) {
	return r.publicKey, nil
}

func (r *syncRepository) OriginSessionMatches(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error) {
	return r.matches, nil
}

func (r *syncRepository) ApplySyncMutation(context.Context, domain.Principal, domain.SyncMutation, []byte) (domain.StoredOperationResult, bool, error) {
	return r.result, r.replayed, r.applyErr
}

func signedMutation(t *testing.T) (domain.SyncMutation, ed25519.PublicKey) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	terminalID := uuid.New()
	mutation := domain.SyncMutation{
		OperationID:     uuid.NewString(),
		Aggregate:       "transaction",
		AggregateID:     "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Action:          "create",
		OriginSessionID: uuid.New(),
		OriginActorID:   uuid.New(),
		TerminalID:      terminalID,
		OccurredAt:      time.Date(2026, 7, 24, 1, 2, 3, 0, time.UTC),
		Payload: json.RawMessage(`{
			"id":"01ARZ3NDEKTSV4RRFFQ69G5FAV",
			"items":[{"packageId":"00000000-0000-4000-8000-000000000001","packageRevision":1,"quantity":1}]
		}`),
	}
	canonical, err := security.CanonicalMutation(mutation)
	if err != nil {
		t.Fatal(err)
	}
	mutation.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical))
	return mutation, publicKey
}

func TestSyncDuplicateReplaysStoredResult(t *testing.T) {
	mutation, publicKey := signedMutation(t)
	repo := &syncRepository{
		publicKey: publicKey, matches: true, replayed: true,
		result: domain.StoredOperationResult{
			Status: 201, Response: json.RawMessage(`{"id":"01ARZ3NDEKTSV4RRFFQ69G5FAV"}`),
		},
	}
	service := Sync{Repo: repo}
	terminalID := mutation.TerminalID
	results, err := service.Push(context.Background(), domain.Principal{
		UserID: uuid.New(), SessionID: uuid.New(), TerminalID: &terminalID,
		Role: domain.RoleAdmin,
	}, []domain.SyncMutation{mutation})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].Replayed || results[0].Error != nil {
		t.Fatalf("unexpected duplicate result: %+v", results)
	}
}

func TestSyncConflictIsReturnedPerOperation(t *testing.T) {
	mutation, publicKey := signedMutation(t)
	conflict := &domain.Error{
		Code: domain.CodeRevisionConflict, Message: "stale",
		Details: map[string]any{"baseRevision": 1, "currentRevision": 2},
	}
	repo := &syncRepository{
		publicKey: publicKey, matches: true, applyErr: conflict,
		result: domain.StoredOperationResult{Status: 409},
	}
	service := Sync{Repo: repo}
	terminalID := mutation.TerminalID
	results, err := service.Push(context.Background(), domain.Principal{
		UserID: uuid.New(), SessionID: uuid.New(), TerminalID: &terminalID,
		Role: domain.RoleAdmin,
	}, []domain.SyncMutation{mutation})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !domain.IsCode(results[0].Error, domain.CodeRevisionConflict) {
		t.Fatalf("expected per-operation conflict: %+v", results)
	}
}
