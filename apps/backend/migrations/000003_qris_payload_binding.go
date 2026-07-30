package migrations

import (
	"context"
	"fmt"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"gorm.io/gorm"
)

// qrisPayloadBindingTransactionModel is scoped to migration 000003 so the
// historical 000001 and 000002 models continue to describe their exact schema.
// NULL remains allowed for QRIS rows created before this binding existed; all
// new application writes require a digest.
type qrisPayloadBindingTransactionModel struct {
	ID              string  `gorm:"column:id;type:text;primaryKey"`
	PaymentMethod   string  `gorm:"column:payment_method;type:text;not null"`
	QrisPayloadHash *string `gorm:"column:qris_payload_hash;type:text;check:transactions_qris_payload_hash_shape,qris_payload_hash IS NULL OR (payment_method = 'qris' AND qris_payload_hash ~ '^[0-9a-f]{64}$')"`
}

func (qrisPayloadBindingTransactionModel) TableName() string { return "transactions" }

type qrisPayloadBindingRevisionModel struct {
	TransactionID   string  `gorm:"column:transaction_id;type:text;primaryKey"`
	Revision        int     `gorm:"column:revision;type:integer;primaryKey;autoIncrement:false"`
	AfterSnapshot   []byte  `gorm:"column:after_snapshot;type:jsonb;not null"`
	QrisPayloadHash *string `gorm:"column:qris_payload_hash;type:text;check:transaction_revisions_qris_payload_hash_shape,qris_payload_hash IS NULL OR ((after_snapshot ->> 'paymentMethod') = 'qris' AND qris_payload_hash ~ '^[0-9a-f]{64}$')"`
}

func (qrisPayloadBindingRevisionModel) TableName() string {
	return "transaction_revisions"
}

func migrateQrisPayloadBinding(ctx context.Context, tx *gorm.DB) error {
	defer observability.StartSegment(ctx, "Migrations.QrisPayloadBinding")()

	for _, addition := range []struct {
		model any
		field string
	}{
		{model: &qrisPayloadBindingTransactionModel{}, field: "QrisPayloadHash"},
		{model: &qrisPayloadBindingRevisionModel{}, field: "QrisPayloadHash"},
	} {
		if tx.Migrator().HasColumn(addition.model, addition.field) {
			continue
		}
		if err := tx.Migrator().AddColumn(addition.model, addition.field); err != nil {
			return fmt.Errorf("add QRIS payload binding column %s: %w", addition.field, err)
		}
	}

	// Both additions are nullable and have no default, so PostgreSQL leaves
	// every pre-binding row unbound. In particular, never backfill
	// transaction_revisions: the table is intentionally append-only and the
	// migration must preserve that invariant while upgrading an existing
	// database.

	for _, constraint := range []struct {
		model any
		name  string
	}{
		{
			model: &qrisPayloadBindingTransactionModel{},
			name:  "transactions_qris_payload_hash_shape",
		},
		{
			model: &qrisPayloadBindingRevisionModel{},
			name:  "transaction_revisions_qris_payload_hash_shape",
		},
	} {
		if tx.Migrator().HasConstraint(constraint.model, constraint.name) {
			continue
		}
		if err := tx.Migrator().CreateConstraint(constraint.model, constraint.name); err != nil {
			return fmt.Errorf("create constraint %s: %w", constraint.name, err)
		}
	}

	return nil
}
