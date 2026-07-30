package postgres

import (
	"testing"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
)

func TestValidatePrintableRevisionRequiresSuccessfulExactCurrentRevision(t *testing.T) {
	t.Parallel()

	revision := 3
	tests := []struct {
		name      string
		requested int
		current   int
		status    domain.PaymentStatus
		confirmed *int
		deleted   bool
		wantCode  string
	}{
		{
			name: "paid current revision", requested: 3, current: 3,
			status: domain.PaymentStatusSuccess, confirmed: &revision,
		},
		{
			name: "pending payment", requested: 3, current: 3,
			status: domain.PaymentStatusPending, wantCode: domain.CodeConflict,
		},
		{
			name: "failed payment", requested: 3, current: 3,
			status: domain.PaymentStatusFailed, wantCode: domain.CodeConflict,
		},
		{
			name: "success confirmed for stale revision", requested: 3, current: 3,
			status: domain.PaymentStatusSuccess, confirmed: intPointer(2),
			wantCode: domain.CodeConflict,
		},
		{
			name: "stale print request", requested: 2, current: 3,
			status: domain.PaymentStatusSuccess, confirmed: &revision,
			wantCode: domain.CodeConflict,
		},
		{
			name: "deleted transaction", requested: 3, current: 3,
			status: domain.PaymentStatusSuccess, confirmed: &revision, deleted: true,
			wantCode: domain.CodeConflict,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validatePrintableRevision(
				test.requested,
				test.current,
				test.status,
				test.confirmed,
				test.deleted,
			)
			if test.wantCode == "" {
				if err != nil {
					t.Fatalf("expected printable revision: %v", err)
				}
				return
			}
			if !domain.IsCode(err, test.wantCode) {
				t.Fatalf("error=%v, want code %s", err, test.wantCode)
			}
		})
	}
}

func intPointer(value int) *int { return &value }
