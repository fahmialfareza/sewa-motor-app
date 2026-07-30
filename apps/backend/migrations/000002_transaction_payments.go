package migrations

import (
	"context"
	"fmt"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"gorm.io/gorm"
)

// transactionPaymentModel is deliberately scoped to migration 000002. The
// already-applied 000001 model remains an immutable description of v1.
type transactionPaymentModel struct {
	ID                       string     `gorm:"column:id;type:text;primaryKey"`
	CurrentRevision          int        `gorm:"column:current_revision;type:integer;not null"`
	OccurredAt               time.Time  `gorm:"column:occurred_at;type:timestamptz;not null;index:transactions_paid_occurred_idx,priority:2,sort:desc,where:deleted_at IS NULL AND payment_status = 'success'"`
	PaymentMethod            string     `gorm:"column:payment_method;type:text;not null;default:'legacy';check:transactions_payment_method_allowed,payment_method IN ('cash','qris','legacy')"`
	PaymentStatus            string     `gorm:"column:payment_status;type:text;not null;default:'pending';index:transactions_paid_occurred_idx,priority:1,where:deleted_at IS NULL AND payment_status = 'success';check:transactions_payment_status_allowed,payment_status IN ('pending','success','failed')"`
	PaymentConfirmedRevision *int       `gorm:"column:payment_confirmed_revision;type:integer;check:transactions_payment_confirmation_shape,(payment_status = 'success' AND payment_confirmed_revision = current_revision) OR (payment_status IN ('pending','failed') AND payment_confirmed_revision IS NULL)"`
	DeletedAt                *time.Time `gorm:"column:deleted_at;type:timestamptz"`
}

func (transactionPaymentModel) TableName() string { return "transactions" }

func migrateTransactionPayments(ctx context.Context, tx *gorm.DB) error {
	defer observability.StartSegment(ctx, "Migrations.TransactionPayments")()

	for _, field := range []string{
		"PaymentMethod",
		"PaymentStatus",
		"PaymentConfirmedRevision",
	} {
		if tx.Migrator().HasColumn(&transactionPaymentModel{}, field) {
			continue
		}
		if err := tx.Migrator().AddColumn(&transactionPaymentModel{}, field); err != nil {
			return fmt.Errorf("add transaction payment column %s: %w", field, err)
		}
	}

	// Before this feature every accepted transaction implicitly represented a
	// successful legacy payment. Preserve reporting and print eligibility while
	// making all new creates explicitly pending.
	if err := tx.Model(&transactionPaymentModel{}).
		Where("payment_method = ? AND payment_status = ?", "legacy", "pending").
		UpdateColumns(map[string]any{
			"payment_status":             "success",
			"payment_confirmed_revision": gorm.Expr("current_revision"),
		}).Error; err != nil {
		return fmt.Errorf("backfill legacy transaction payments: %w", err)
	}
	for _, constraint := range []string{
		"transactions_payment_method_allowed",
		"transactions_payment_status_allowed",
		"transactions_payment_confirmation_shape",
	} {
		if tx.Migrator().HasConstraint(&transactionPaymentModel{}, constraint) {
			continue
		}
		if err := tx.Migrator().CreateConstraint(&transactionPaymentModel{}, constraint); err != nil {
			return fmt.Errorf("create constraint %s: %w", constraint, err)
		}
	}

	if !tx.Migrator().HasIndex(&transactionPaymentModel{}, "transactions_paid_occurred_idx") {
		if err := tx.Migrator().CreateIndex(&transactionPaymentModel{}, "transactions_paid_occurred_idx"); err != nil {
			return fmt.Errorf("create successful payment index: %w", err)
		}
	}

	return nil
}
