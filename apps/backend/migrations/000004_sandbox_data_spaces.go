package migrations

import (
	"context"
	"fmt"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"gorm.io/gorm"
)

// migrateSandboxDataSpaces adds a logical data-plane boundary to the existing
// production database. Constant DEFAULT values deliberately backfill the
// append-only tables through DDL; no UPDATE is issued against their rows.
func migrateSandboxDataSpaces(ctx context.Context, tx *gorm.DB) error {
	defer observability.StartSegment(ctx, "Migrations.SandboxDataSpaces")()

	// A fresh installation applies every migration in one transaction. The
	// initial package seed leaves deferred circular-FK trigger events pending,
	// and PostgreSQL refuses ALTER TABLE while those events are outstanding.
	// Validate them now, then restore deferred mode for the circular current-
	// revision constraints used by the package clone later in this migration.
	if err := tx.WithContext(ctx).Exec(`SET CONSTRAINTS ALL IMMEDIATE`).Error; err != nil {
		return fmt.Errorf("validate pending constraints before sandbox schema: %w", err)
	}
	if err := tx.WithContext(ctx).Exec(`
		SET CONSTRAINTS packages_current_revision_fk, transactions_current_revision_fk DEFERRED
	`).Error; err != nil {
		return fmt.Errorf("defer circular revision constraints for sandbox schema: %w", err)
	}

	liveID := domain.LiveDataSpaceIDString
	statements := []string{
		`CREATE TABLE data_spaces (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			mode text NOT NULL CHECK (mode IN ('production','sandbox')),
			generation bigint NOT NULL CHECK (generation > 0),
			status text NOT NULL CHECK (status IN ('active','retired','purged')),
			activated_at timestamptz NOT NULL DEFAULT now(),
			retired_at timestamptz,
			purge_after timestamptz,
			purged_at timestamptz,
			CONSTRAINT data_spaces_mode_generation_key UNIQUE (mode, generation),
			CONSTRAINT data_spaces_lifecycle_shape CHECK (
				(status = 'active' AND retired_at IS NULL AND purge_after IS NULL AND purged_at IS NULL) OR
				(status = 'retired' AND retired_at IS NOT NULL AND purge_after IS NOT NULL AND purged_at IS NULL) OR
				(status = 'purged' AND retired_at IS NOT NULL AND purge_after IS NOT NULL AND purged_at IS NOT NULL)
			)
		)`,
		`CREATE UNIQUE INDEX data_spaces_active_mode_key ON data_spaces (mode) WHERE status = 'active'`,
		fmt.Sprintf(`INSERT INTO data_spaces (id, mode, generation, status)
			VALUES ('%s', 'production', 1, 'active')`, liveID),
	}

	for _, table := range []string{
		"sessions", "packages", "package_revisions", "transactions",
		"transaction_revisions", "transaction_items", "print_attempts",
		"audit_events", "sync_changes", "idempotency_records",
	} {
		statements = append(statements, fmt.Sprintf(
			`ALTER TABLE %s ADD COLUMN data_space_id uuid NOT NULL DEFAULT '%s'`,
			table, liveID,
		))
	}
	statements = append(statements,
		`ALTER TABLE packages ADD COLUMN source_package_id uuid`,
		`ALTER TABLE packages ADD COLUMN source_revision integer`,
		`ALTER TABLE packages ADD CONSTRAINT packages_source_shape CHECK (
			(source_package_id IS NULL AND source_revision IS NULL) OR
			(source_package_id IS NOT NULL AND source_revision > 0)
		)`,
		`ALTER TABLE transactions ADD COLUMN payment_amount bigint`,
		`UPDATE transactions SET payment_amount = total WHERE payment_amount IS NULL`,
		`ALTER TABLE transactions ALTER COLUMN payment_amount SET NOT NULL`,
		`ALTER TABLE transactions ADD CONSTRAINT transactions_payment_amount_nonnegative CHECK (payment_amount >= 0)`,
		// Keep pre-data-space replicas write-compatible throughout the additive
		// rollout. They omit payment_amount on create and only change total during
		// correction. The production-only guard deliberately leaves Sandbox's
		// fixed QRIS amount under the scope-aware backend's control. Remove this
		// trigger only in a later migration after old replicas are impossible.
		fmt.Sprintf(`CREATE OR REPLACE FUNCTION maintain_legacy_production_payment_amount()
			RETURNS trigger
			LANGUAGE plpgsql
			AS $$
			BEGIN
				IF NEW.data_space_id = '%s'::uuid THEN
					IF TG_OP = 'INSERT' AND NEW.payment_amount IS NULL THEN
						NEW.payment_amount := NEW.total;
					ELSIF TG_OP = 'UPDATE'
					   AND NEW.total IS DISTINCT FROM OLD.total
					   AND NEW.payment_amount IS NOT DISTINCT FROM OLD.payment_amount THEN
						NEW.payment_amount := NEW.total;
					END IF;
				END IF;
				RETURN NEW;
			END
			$$`, liveID),
		`CREATE TRIGGER transactions_legacy_production_payment_amount
			BEFORE INSERT OR UPDATE OF total, payment_amount, data_space_id ON transactions
			FOR EACH ROW EXECUTE FUNCTION maintain_legacy_production_payment_amount()`,
		// Historical revision rows remain untouched. Read paths derive the old
		// amount from after_snapshot/transaction total when this column is NULL.
		`ALTER TABLE transaction_revisions ADD COLUMN payment_amount bigint`,
		`ALTER TABLE transaction_revisions ADD CONSTRAINT transaction_revisions_payment_amount_nonnegative CHECK (payment_amount IS NULL OR payment_amount >= 0)`,
		`DROP INDEX packages_code_key`,
		`CREATE UNIQUE INDEX packages_data_space_code_key ON packages (data_space_id, code)`,
		`ALTER TABLE idempotency_records DROP CONSTRAINT idempotency_records_pkey`,
		`ALTER TABLE idempotency_records ADD CONSTRAINT idempotency_records_pkey PRIMARY KEY (data_space_id, terminal_id, operation_id)`,
	)

	for _, table := range []string{
		"sessions", "packages", "package_revisions", "transactions",
		"transaction_revisions", "transaction_items", "print_attempts",
		"audit_events", "sync_changes", "idempotency_records",
	} {
		statements = append(statements, fmt.Sprintf(
			`ALTER TABLE %s ADD CONSTRAINT %s_data_space_id_fkey FOREIGN KEY (data_space_id) REFERENCES data_spaces(id)`,
			table, table,
		))
	}

	statements = append(statements,
		`ALTER TABLE sessions ADD CONSTRAINT sessions_id_data_space_key UNIQUE (id, data_space_id)`,
		`ALTER TABLE packages ADD CONSTRAINT packages_id_data_space_key UNIQUE (id, data_space_id)`,
		`ALTER TABLE package_revisions ADD CONSTRAINT package_revisions_space_key UNIQUE (package_id, revision, data_space_id)`,
		`ALTER TABLE transactions ADD CONSTRAINT transactions_id_data_space_key UNIQUE (id, data_space_id)`,
		`ALTER TABLE transaction_revisions ADD CONSTRAINT transaction_revisions_space_key UNIQUE (transaction_id, revision, data_space_id)`,
		`ALTER TABLE package_revisions ADD CONSTRAINT package_revisions_package_space_fkey FOREIGN KEY (package_id, data_space_id) REFERENCES packages(id, data_space_id)`,
		`ALTER TABLE packages ADD CONSTRAINT packages_current_revision_space_fkey FOREIGN KEY (id, current_revision, data_space_id) REFERENCES package_revisions(package_id, revision, data_space_id) DEFERRABLE INITIALLY DEFERRED`,
		`ALTER TABLE transactions ADD CONSTRAINT transactions_origin_session_space_fkey FOREIGN KEY (origin_session_id, data_space_id) REFERENCES sessions(id, data_space_id)`,
		`ALTER TABLE transaction_revisions ADD CONSTRAINT transaction_revisions_transaction_space_fkey FOREIGN KEY (transaction_id, data_space_id) REFERENCES transactions(id, data_space_id)`,
		`ALTER TABLE transaction_revisions ADD CONSTRAINT transaction_revisions_origin_session_space_fkey FOREIGN KEY (origin_session_id, data_space_id) REFERENCES sessions(id, data_space_id)`,
		`ALTER TABLE transaction_revisions ADD CONSTRAINT transaction_revisions_submitted_session_space_fkey FOREIGN KEY (submitted_by_session_id, data_space_id) REFERENCES sessions(id, data_space_id)`,
		`ALTER TABLE transactions ADD CONSTRAINT transactions_current_revision_space_fkey FOREIGN KEY (id, current_revision, data_space_id) REFERENCES transaction_revisions(transaction_id, revision, data_space_id) DEFERRABLE INITIALLY DEFERRED`,
		`ALTER TABLE transaction_items ADD CONSTRAINT transaction_items_transaction_revision_space_fkey FOREIGN KEY (transaction_id, revision, data_space_id) REFERENCES transaction_revisions(transaction_id, revision, data_space_id)`,
		`ALTER TABLE transaction_items ADD CONSTRAINT transaction_items_package_revision_space_fkey FOREIGN KEY (package_id, package_revision, data_space_id) REFERENCES package_revisions(package_id, revision, data_space_id)`,
		`ALTER TABLE print_attempts ADD CONSTRAINT print_attempts_transaction_revision_space_fkey FOREIGN KEY (transaction_id, transaction_revision, data_space_id) REFERENCES transaction_revisions(transaction_id, revision, data_space_id)`,
		`ALTER TABLE print_attempts ADD CONSTRAINT print_attempts_session_space_fkey FOREIGN KEY (session_id, data_space_id) REFERENCES sessions(id, data_space_id)`,
		`ALTER TABLE audit_events ADD CONSTRAINT audit_events_origin_session_space_fkey FOREIGN KEY (origin_session_id, data_space_id) REFERENCES sessions(id, data_space_id)`,
		`ALTER TABLE audit_events ADD CONSTRAINT audit_events_submitted_session_space_fkey FOREIGN KEY (submitted_by_session_id, data_space_id) REFERENCES sessions(id, data_space_id)`,
		`ALTER TABLE packages ADD CONSTRAINT packages_source_package_id_fkey FOREIGN KEY (source_package_id) REFERENCES packages(id)`,
		`CREATE INDEX sessions_data_space_live_idx ON sessions (data_space_id, user_id) WHERE revoked_at IS NULL`,
		`CREATE INDEX packages_data_space_live_idx ON packages (data_space_id, updated_at DESC) WHERE deleted_at IS NULL`,
		`CREATE INDEX transactions_data_space_occurred_idx ON transactions (data_space_id, occurred_at DESC, id DESC)`,
		`CREATE INDEX sync_changes_data_space_cursor_idx ON sync_changes (data_space_id, cursor)`,
		`CREATE INDEX audit_events_data_space_idx ON audit_events (data_space_id, server_received_at DESC)`,
	)

	for _, statement := range statements {
		if err := tx.WithContext(ctx).Exec(statement).Error; err != nil {
			return fmt.Errorf("apply sandbox data-space schema: %w", err)
		}
	}
	// The normal invariant remains append-only. Cleanup can only delete rows
	// whose own data_space_id matches an expired, retired Sandbox generation.
	if err := tx.WithContext(ctx).Exec(`
		CREATE OR REPLACE FUNCTION reject_append_only_mutation()
		RETURNS trigger
		LANGUAGE plpgsql
		AS $$
		DECLARE purge_space text;
		BEGIN
			purge_space := current_setting('app.sandbox_purge_data_space_id', true);
			IF TG_OP = 'DELETE' AND purge_space IS NOT NULL
			   AND (to_jsonb(OLD) ->> 'data_space_id') = purge_space
			   AND EXISTS (
				SELECT 1 FROM data_spaces
				WHERE id = purge_space::uuid AND mode = 'sandbox'
				  AND status = 'retired' AND purge_after <= now()
			   ) THEN
				RETURN OLD;
			END IF;
			RAISE EXCEPTION '% is append-only', TG_TABLE_NAME USING ERRCODE = '55000';
		END
		$$
	`).Error; err != nil {
		return fmt.Errorf("install scoped append-only purge guard: %w", err)
	}

	return nil
}
