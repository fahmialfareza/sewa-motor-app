package migrations

import (
	"context"
	"fmt"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type foreignKeySpec struct {
	model     any
	name      string
	statement string
}

var initialForeignKeys = []foreignKeySpec{
	{&terminalModel{}, "terminals_enrolled_by_fkey", `ALTER TABLE terminals ADD CONSTRAINT terminals_enrolled_by_fkey FOREIGN KEY (enrolled_by) REFERENCES users(id)`},
	{&sessionModel{}, "sessions_user_id_fkey", `ALTER TABLE sessions ADD CONSTRAINT sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id)`},
	{&sessionModel{}, "sessions_terminal_id_fkey", `ALTER TABLE sessions ADD CONSTRAINT sessions_terminal_id_fkey FOREIGN KEY (terminal_id) REFERENCES terminals(id)`},
	{&packageModel{}, "packages_created_by_fkey", `ALTER TABLE packages ADD CONSTRAINT packages_created_by_fkey FOREIGN KEY (created_by) REFERENCES users(id)`},
	{&packageModel{}, "packages_updated_by_fkey", `ALTER TABLE packages ADD CONSTRAINT packages_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES users(id)`},
	{&packageModel{}, "packages_deleted_by_fkey", `ALTER TABLE packages ADD CONSTRAINT packages_deleted_by_fkey FOREIGN KEY (deleted_by) REFERENCES users(id)`},
	{&packageRevisionModel{}, "package_revisions_package_id_fkey", `ALTER TABLE package_revisions ADD CONSTRAINT package_revisions_package_id_fkey FOREIGN KEY (package_id) REFERENCES packages(id)`},
	{&packageRevisionModel{}, "package_revisions_created_by_fkey", `ALTER TABLE package_revisions ADD CONSTRAINT package_revisions_created_by_fkey FOREIGN KEY (created_by) REFERENCES users(id)`},
	{&packageModel{}, "packages_current_revision_fk", `ALTER TABLE packages ADD CONSTRAINT packages_current_revision_fk FOREIGN KEY (id, current_revision) REFERENCES package_revisions(package_id, revision) DEFERRABLE INITIALLY DEFERRED`},
	{&transactionModel{}, "transactions_origin_actor_id_fkey", `ALTER TABLE transactions ADD CONSTRAINT transactions_origin_actor_id_fkey FOREIGN KEY (origin_actor_id) REFERENCES users(id)`},
	{&transactionModel{}, "transactions_origin_session_id_fkey", `ALTER TABLE transactions ADD CONSTRAINT transactions_origin_session_id_fkey FOREIGN KEY (origin_session_id) REFERENCES sessions(id)`},
	{&transactionModel{}, "transactions_terminal_id_fkey", `ALTER TABLE transactions ADD CONSTRAINT transactions_terminal_id_fkey FOREIGN KEY (terminal_id) REFERENCES terminals(id)`},
	{&transactionModel{}, "transactions_updated_by_fkey", `ALTER TABLE transactions ADD CONSTRAINT transactions_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES users(id)`},
	{&transactionModel{}, "transactions_deleted_by_fkey", `ALTER TABLE transactions ADD CONSTRAINT transactions_deleted_by_fkey FOREIGN KEY (deleted_by) REFERENCES users(id)`},
	{&transactionRevisionModel{}, "transaction_revisions_transaction_id_fkey", `ALTER TABLE transaction_revisions ADD CONSTRAINT transaction_revisions_transaction_id_fkey FOREIGN KEY (transaction_id) REFERENCES transactions(id)`},
	{&transactionRevisionModel{}, "transaction_revisions_origin_actor_id_fkey", `ALTER TABLE transaction_revisions ADD CONSTRAINT transaction_revisions_origin_actor_id_fkey FOREIGN KEY (origin_actor_id) REFERENCES users(id)`},
	{&transactionRevisionModel{}, "transaction_revisions_origin_session_id_fkey", `ALTER TABLE transaction_revisions ADD CONSTRAINT transaction_revisions_origin_session_id_fkey FOREIGN KEY (origin_session_id) REFERENCES sessions(id)`},
	{&transactionRevisionModel{}, "transaction_revisions_terminal_id_fkey", `ALTER TABLE transaction_revisions ADD CONSTRAINT transaction_revisions_terminal_id_fkey FOREIGN KEY (terminal_id) REFERENCES terminals(id)`},
	{&transactionRevisionModel{}, "transaction_revisions_submitted_by_actor_id_fkey", `ALTER TABLE transaction_revisions ADD CONSTRAINT transaction_revisions_submitted_by_actor_id_fkey FOREIGN KEY (submitted_by_actor_id) REFERENCES users(id)`},
	{&transactionRevisionModel{}, "transaction_revisions_submitted_by_session_id_fkey", `ALTER TABLE transaction_revisions ADD CONSTRAINT transaction_revisions_submitted_by_session_id_fkey FOREIGN KEY (submitted_by_session_id) REFERENCES sessions(id)`},
	{&transactionModel{}, "transactions_current_revision_fk", `ALTER TABLE transactions ADD CONSTRAINT transactions_current_revision_fk FOREIGN KEY (id, current_revision) REFERENCES transaction_revisions(transaction_id, revision) DEFERRABLE INITIALLY DEFERRED`},
	{&transactionItemModel{}, "transaction_items_transaction_revision_fkey", `ALTER TABLE transaction_items ADD CONSTRAINT transaction_items_transaction_revision_fkey FOREIGN KEY (transaction_id, revision) REFERENCES transaction_revisions(transaction_id, revision)`},
	{&transactionItemModel{}, "transaction_items_package_revision_fkey", `ALTER TABLE transaction_items ADD CONSTRAINT transaction_items_package_revision_fkey FOREIGN KEY (package_id, package_revision) REFERENCES package_revisions(package_id, revision)`},
	{&printAttemptModel{}, "print_attempts_transaction_revision_fkey", `ALTER TABLE print_attempts ADD CONSTRAINT print_attempts_transaction_revision_fkey FOREIGN KEY (transaction_id, transaction_revision) REFERENCES transaction_revisions(transaction_id, revision)`},
	{&printAttemptModel{}, "print_attempts_terminal_id_fkey", `ALTER TABLE print_attempts ADD CONSTRAINT print_attempts_terminal_id_fkey FOREIGN KEY (terminal_id) REFERENCES terminals(id)`},
	{&printAttemptModel{}, "print_attempts_actor_id_fkey", `ALTER TABLE print_attempts ADD CONSTRAINT print_attempts_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES users(id)`},
	{&printAttemptModel{}, "print_attempts_session_id_fkey", `ALTER TABLE print_attempts ADD CONSTRAINT print_attempts_session_id_fkey FOREIGN KEY (session_id) REFERENCES sessions(id)`},
	{&auditEventModel{}, "audit_events_origin_actor_id_fkey", `ALTER TABLE audit_events ADD CONSTRAINT audit_events_origin_actor_id_fkey FOREIGN KEY (origin_actor_id) REFERENCES users(id)`},
	{&auditEventModel{}, "audit_events_origin_session_id_fkey", `ALTER TABLE audit_events ADD CONSTRAINT audit_events_origin_session_id_fkey FOREIGN KEY (origin_session_id) REFERENCES sessions(id)`},
	{&auditEventModel{}, "audit_events_submitted_by_actor_id_fkey", `ALTER TABLE audit_events ADD CONSTRAINT audit_events_submitted_by_actor_id_fkey FOREIGN KEY (submitted_by_actor_id) REFERENCES users(id)`},
	{&auditEventModel{}, "audit_events_submitted_by_session_id_fkey", `ALTER TABLE audit_events ADD CONSTRAINT audit_events_submitted_by_session_id_fkey FOREIGN KEY (submitted_by_session_id) REFERENCES sessions(id)`},
	{&auditEventModel{}, "audit_events_terminal_id_fkey", `ALTER TABLE audit_events ADD CONSTRAINT audit_events_terminal_id_fkey FOREIGN KEY (terminal_id) REFERENCES terminals(id)`},
	{&idempotencyRecordModel{}, "idempotency_records_terminal_id_fkey", `ALTER TABLE idempotency_records ADD CONSTRAINT idempotency_records_terminal_id_fkey FOREIGN KEY (terminal_id) REFERENCES terminals(id)`},
}

var appendOnlyTables = []string{
	"package_revisions",
	"transaction_revisions",
	"transaction_items",
	"print_attempts",
	"audit_events",
	"sync_changes",
	"idempotency_records",
}

const (
	standardPackageID = "00000000-0000-4000-8000-000000000001"
	sunrisePackageID  = "00000000-0000-4000-8000-000000000002"
)

func migrateInitialSchema(ctx context.Context, tx *gorm.DB) error {
	defer observability.StartSegment(ctx, "Migrations.Initial")()

	if err := tx.Exec(`CREATE EXTENSION IF NOT EXISTS pgcrypto`).Error; err != nil {
		return fmt.Errorf("create pgcrypto extension: %w", err)
	}
	if err := tx.AutoMigrate(initialSchemaModels()...); err != nil {
		return fmt.Errorf("auto migrate initial models: %w", err)
	}
	if err := addForeignKeys(ctx, tx); err != nil {
		return err
	}
	if err := installAppendOnlyGuards(ctx, tx); err != nil {
		return err
	}
	if err := seedPackages(ctx, tx); err != nil {
		return err
	}
	return nil
}

func addForeignKeys(ctx context.Context, tx *gorm.DB) error {
	defer observability.StartSegment(ctx, "Migrations.Initial.ForeignKeys")()

	for _, spec := range initialForeignKeys {
		if tx.Migrator().HasConstraint(spec.model, spec.name) {
			continue
		}
		if err := tx.Exec(spec.statement).Error; err != nil {
			return fmt.Errorf("create foreign key %s: %w", spec.name, err)
		}
	}
	return nil
}

func installAppendOnlyGuards(ctx context.Context, tx *gorm.DB) error {
	defer observability.StartSegment(ctx, "Migrations.Initial.AppendOnlyGuards")()

	if err := tx.Exec(`
		CREATE OR REPLACE FUNCTION reject_append_only_mutation()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		BEGIN
			RAISE EXCEPTION '% is append-only', TG_TABLE_NAME
				USING ERRCODE = '55000';
		END
		$$
	`).Error; err != nil {
		return fmt.Errorf("create append-only function: %w", err)
	}

	for _, table := range appendOnlyTables {
		statement := fmt.Sprintf(`
			CREATE OR REPLACE TRIGGER %s_append_only
			BEFORE UPDATE OR DELETE ON %s
			FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation()
		`, table, table)
		if err := tx.Exec(statement).Error; err != nil {
			return fmt.Errorf("create append-only trigger for %s: %w", table, err)
		}
	}
	return nil
}

func seedPackages(ctx context.Context, tx *gorm.DB) error {
	defer observability.StartSegment(ctx, "Migrations.Initial.SeedPackages")()

	packages := []packageModel{
		{
			ID:              uuid.MustParse(standardPackageID),
			Code:            "STANDARD",
			CurrentRevision: 1,
		},
		{
			ID:              uuid.MustParse(sunrisePackageID),
			Code:            "SUNRISE",
			CurrentRevision: 1,
		},
	}
	if err := tx.
		Omit("CreatedAt", "UpdatedAt").
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoNothing: true,
		}).
		Create(&packages).Error; err != nil {
		return fmt.Errorf("seed packages: %w", err)
	}

	reason := "Paket awal sistem"
	revisions := []packageRevisionModel{
		{
			PackageID:    packages[0].ID,
			Revision:     1,
			Name:         "Paket Standar",
			Description:  "Paket sewa motor standar",
			UnitPrice:    70_000,
			ChangeReason: &reason,
		},
		{
			PackageID:    packages[1].ID,
			Revision:     1,
			Name:         "Paket Sunrise",
			Description:  "Paket sewa motor sunrise",
			UnitPrice:    100_000,
			ChangeReason: &reason,
		},
	}
	if err := tx.
		Omit("CreatedAt").
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "package_id"},
				{Name: "revision"},
			},
			DoNothing: true,
		}).
		Create(&revisions).Error; err != nil {
		return fmt.Errorf("seed package revisions: %w", err)
	}
	return nil
}
