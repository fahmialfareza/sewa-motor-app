package postgres

import (
	"strings"
	"testing"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
)

func TestExportQueryDoesNotImplicitlyHideUnsuccessfulPayments(t *testing.T) {
	t.Parallel()

	conditions, args := exportQueryConditions(domain.TransactionFilter{DataSpaceID: domain.LiveDataSpaceID()})
	query := strings.Join(conditions, " AND ")
	if strings.Contains(query, "payment_status") ||
		strings.Contains(query, "payment_confirmed_revision") {
		t.Fatalf("unfiltered export unexpectedly applies paid-only semantics: %s", query)
	}
	if !strings.Contains(query, "t.data_space_id = $1") ||
		!strings.Contains(query, "deleted_at IS NULL") ||
		len(args) != 1 ||
		args[0] != domain.LiveDataSpaceID() {
		t.Fatalf("unexpected default export conditions=%q args=%v", query, args)
	}

	failed := domain.PaymentStatusFailed
	conditions, args = exportQueryConditions(domain.TransactionFilter{
		DataSpaceID:    domain.LiveDataSpaceID(),
		PaymentStatus:  &failed,
		IncludeDeleted: true,
	})
	query = strings.Join(conditions, " AND ")
	if !strings.Contains(query, "t.data_space_id = $1") ||
		!strings.Contains(query, "t.payment_status = $2") ||
		strings.Contains(query, "deleted_at IS NULL") ||
		len(args) != 2 ||
		args[0] != domain.LiveDataSpaceID() ||
		args[1] != domain.PaymentStatusFailed {
		t.Fatalf("failed-payment export conditions=%q args=%v", query, args)
	}
}

func TestExportQueryAcceptsSandboxDisplayID(t *testing.T) {
	t.Parallel()

	conditions, args := exportQueryConditions(domain.TransactionFilter{
		Search: "TEST-TRX-01ARZ3NDEKTSV4RRFFQ69G5FAV",
	})
	query := strings.Join(conditions, " AND ")
	if !strings.Contains(query, "t.id LIKE $2") || len(args) != 2 ||
		args[1] != "01ARZ3NDEKTSV4RRFFQ69G5FAV%" {
		t.Fatalf("Sandbox display-ID export conditions=%q args=%v", query, args)
	}
}
