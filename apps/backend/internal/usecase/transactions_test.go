package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/port"
	"github.com/google/uuid"
)

const correctionTestTransactionID = "01ARZ3NDEKTSV4RRFFQ69G5FAV"

type correctionRepository struct {
	port.Repository
	transaction   domain.Transaction
	correctCalled bool
}

func (r *correctionRepository) GetTransaction(
	context.Context,
	string,
	bool,
) (domain.Transaction, error) {
	return r.transaction, nil
}

func (r *correctionRepository) CorrectTransaction(
	_ context.Context,
	input domain.CorrectTransactionInput,
) (domain.Transaction, error) {
	r.correctCalled = true
	return domain.Transaction{
		ID:          input.ID,
		Revision:    input.BaseRevision + 1,
		OriginActor: r.transaction.OriginActor,
	}, nil
}

func correctionInput() domain.CorrectTransactionInput {
	return domain.CorrectTransactionInput{
		ID:           correctionTestTransactionID,
		BaseRevision: 1,
		Reason:       "Jumlah paket diperbaiki",
		OccurredAt:   time.Date(2026, 7, 28, 1, 2, 3, 0, time.UTC),
		Items: []domain.ItemInput{{
			PackageID:       uuid.MustParse("00000000-0000-4000-8000-000000000001"),
			PackageRevision: 1,
			Quantity:        1,
		}},
	}
}

func correctionPrincipal(userID, terminalID uuid.UUID, role domain.Role) domain.Principal {
	return domain.Principal{
		UserID:     userID,
		SessionID:  uuid.New(),
		TerminalID: &terminalID,
		Role:       role,
	}
}

func TestTransactionsCorrectAuthorization(t *testing.T) {
	ownerID := uuid.New()
	otherID := uuid.New()
	terminalID := uuid.New()

	tests := []struct {
		name          string
		principal     domain.Principal
		wantForbidden bool
	}{
		{
			name:      "admin owns transaction",
			principal: correctionPrincipal(ownerID, terminalID, domain.RoleAdmin),
		},
		{
			name:          "admin does not own transaction",
			principal:     correctionPrincipal(otherID, terminalID, domain.RoleAdmin),
			wantForbidden: true,
		},
		{
			name:      "superadmin does not need ownership",
			principal: correctionPrincipal(otherID, terminalID, domain.RoleSuperadmin),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &correctionRepository{
				transaction: domain.Transaction{
					ID: correctionTestTransactionID,
					OriginActor: domain.ActorSummary{
						ID: ownerID,
					},
				},
			}
			service := Transactions{Repo: repo}

			_, err := service.Correct(
				context.Background(),
				test.principal,
				correctionInput(),
			)

			if test.wantForbidden {
				if !domain.IsCode(err, domain.CodeForbidden) {
					t.Fatalf("expected forbidden, got %v", err)
				}
				if repo.correctCalled {
					t.Fatal("repository mutation was called for a forbidden correction")
				}
				return
			}
			if err != nil {
				t.Fatalf("expected correction to be allowed: %v", err)
			}
			if !repo.correctCalled {
				t.Fatal("repository mutation was not called")
			}
		})
	}
}
