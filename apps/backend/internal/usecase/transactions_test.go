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
	paymentCalled bool
}

func (r *correctionRepository) SetTransactionPaymentStatus(
	_ context.Context,
	input domain.SetPaymentStatusInput,
) (domain.Transaction, error) {
	r.paymentCalled = true
	result := r.transaction
	result.PaymentStatus = input.Status
	if input.Status == domain.PaymentStatusSuccess {
		result.PaymentConfirmedRevision = &input.BaseRevision
	}
	return result, nil
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
		ID:            correctionTestTransactionID,
		BaseRevision:  1,
		Reason:        "Jumlah paket diperbaiki",
		OccurredAt:    time.Date(2026, 7, 28, 1, 2, 3, 0, time.UTC),
		PaymentMethod: domain.PaymentMethodCash,
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

func TestTransactionsCorrectRequiresQrisPayloadBinding(t *testing.T) {
	t.Parallel()

	ownerID := uuid.New()
	terminalID := uuid.New()
	repo := &correctionRepository{
		transaction: domain.Transaction{
			ID:          correctionTestTransactionID,
			OriginActor: domain.ActorSummary{ID: ownerID},
		},
	}
	service := Transactions{Repo: repo}
	input := correctionInput()
	input.PaymentMethod = domain.PaymentMethodQRIS

	if _, err := service.Correct(
		context.Background(),
		correctionPrincipal(ownerID, terminalID, domain.RoleAdmin),
		input,
	); !domain.IsCode(err, domain.CodeValidation) {
		t.Fatalf("missing QRIS payload hash was not rejected: %v", err)
	}
	if repo.correctCalled {
		t.Fatal("repository mutation was called without a QRIS payload binding")
	}

	hash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	input.QrisPayloadHash = &hash
	if _, err := service.Correct(
		context.Background(),
		correctionPrincipal(ownerID, terminalID, domain.RoleAdmin),
		input,
	); err != nil {
		t.Fatalf("valid QRIS payload binding was rejected: %v", err)
	}
	if !repo.correctCalled {
		t.Fatal("repository mutation was not called with a valid QRIS payload binding")
	}
}

func TestTransactionsSetPaymentStatusAuthorization(t *testing.T) {
	ownerID := uuid.New()
	otherID := uuid.New()
	terminalID := uuid.New()

	tests := []struct {
		name          string
		principal     domain.Principal
		wantForbidden bool
	}{
		{
			name:      "owner admin",
			principal: correctionPrincipal(ownerID, terminalID, domain.RoleAdmin),
		},
		{
			name:          "different admin",
			principal:     correctionPrincipal(otherID, terminalID, domain.RoleAdmin),
			wantForbidden: true,
		},
		{
			name:      "superadmin",
			principal: correctionPrincipal(otherID, terminalID, domain.RoleSuperadmin),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &correctionRepository{
				transaction: domain.Transaction{
					ID:            correctionTestTransactionID,
					Revision:      1,
					PaymentStatus: domain.PaymentStatusPending,
					OriginActor:   domain.ActorSummary{ID: ownerID},
				},
			}
			service := Transactions{Repo: repo}
			_, err := service.SetPaymentStatus(
				context.Background(),
				test.principal,
				domain.SetPaymentStatusInput{
					ID: correctionTestTransactionID, BaseRevision: 1,
					Status:     domain.PaymentStatusSuccess,
					OccurredAt: time.Date(2026, 7, 29, 1, 2, 3, 0, time.UTC),
				},
			)
			if test.wantForbidden {
				if !domain.IsCode(err, domain.CodeForbidden) {
					t.Fatalf("expected forbidden, got %v", err)
				}
				if repo.paymentCalled {
					t.Fatal("payment mutation was called for a forbidden actor")
				}
				return
			}
			if err != nil {
				t.Fatalf("expected payment update to be allowed: %v", err)
			}
			if !repo.paymentCalled {
				t.Fatal("payment mutation was not called")
			}
		})
	}
}
