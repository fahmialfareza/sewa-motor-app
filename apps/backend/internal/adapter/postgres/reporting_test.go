package postgres

import (
	"strings"
	"testing"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
)

func TestExportQueryDoesNotImplicitlyHideUnsuccessfulPayments(t *testing.T) {
	t.Parallel()

	conditions, args := exportQueryConditions(domain.TransactionFilter{})
	query := strings.Join(conditions, " AND ")
	if strings.Contains(query, "payment_status") ||
		strings.Contains(query, "payment_confirmed_revision") {
		t.Fatalf("unfiltered export unexpectedly applies paid-only semantics: %s", query)
	}
	if !strings.Contains(query, "deleted_at IS NULL") || len(args) != 0 {
		t.Fatalf("unexpected default export conditions=%q args=%v", query, args)
	}

	failed := domain.PaymentStatusFailed
	conditions, args = exportQueryConditions(domain.TransactionFilter{
		PaymentStatus:  &failed,
		IncludeDeleted: true,
	})
	query = strings.Join(conditions, " AND ")
	if !strings.Contains(query, "t.payment_status = $1") ||
		strings.Contains(query, "deleted_at IS NULL") ||
		len(args) != 1 ||
		args[0] != domain.PaymentStatusFailed {
		t.Fatalf("failed-payment export conditions=%q args=%v", query, args)
	}
}
