package migrations

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestSandboxMigrationBackfillsAppendOnlyRowsAndRejectsCrossSpaceReferences(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not configured")
	}

	base, err := gorm.Open(gormpostgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	schemaName := "sandbox_migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := `"` + schemaName + `"`
	if err = base.Exec("CREATE SCHEMA " + quotedSchema).Error; err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		if dropErr := base.Exec("DROP SCHEMA " + quotedSchema + " CASCADE").Error; dropErr != nil {
			t.Errorf("drop test schema: %v", dropErr)
		}
		if sqlDB, dbErr := base.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	scopedURL := sandboxScopedDatabaseURL(t, databaseURL, schemaName)
	db, err := gorm.Open(gormpostgres.Open(scopedURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open scoped integration database: %v", err)
	}
	if sqlDB, dbErr := db.DB(); dbErr == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	for _, migration := range []func(context.Context, *gorm.DB) error{
		migrateInitialSchema,
		migrateTransactionPayments,
		migrateQrisPayloadBinding,
	} {
		if err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return migration(ctx, tx)
		}); err != nil {
			t.Fatalf("apply prerequisite migration: %v", err)
		}
	}

	userID := uuid.New()
	terminalID := uuid.New()
	sessionID := uuid.New()
	transactionID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	if err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		statements := []struct {
			query string
			args  []any
		}{
			{
				query: `INSERT INTO users (
					id, full_name, username, password_hash, role, is_active, must_change_password
				) VALUES (?, 'Sandbox Migration Admin', ?, 'hash', 'superadmin', true, false)`,
				args: []any{userID, "sandbox_migration_admin"},
			},
			{
				query: `INSERT INTO terminals (
					id, installation_id, name, public_key, enrolled_by
				) VALUES (?, ?, 'Sandbox Migration Terminal', ?, ?)`,
				args: []any{terminalID, uuid.NewString(), make([]byte, 32), userID},
			},
			{
				query: `INSERT INTO sessions (id, user_id, terminal_id, token_hash)
					VALUES (?, ?, ?, ?)`,
				args: []any{sessionID, userID, terminalID, bytesOf(1, 32)},
			},
			{
				query: `INSERT INTO transactions (
					id, current_revision, occurred_at, origin_actor_id, origin_session_id,
					terminal_id, updated_by, subtotal, total, payment_method, payment_status
				) VALUES (?, 1, now(), ?, ?, ?, ?, 70000, 70000, 'cash', 'pending')`,
				args: []any{transactionID, userID, sessionID, terminalID, userID},
			},
			{
				query: `INSERT INTO transaction_revisions (
					transaction_id, revision, change_type, after_snapshot,
					origin_actor_id, origin_session_id, terminal_id,
					submitted_by_actor_id, submitted_by_session_id, client_occurred_at
				) VALUES (
					?, 1, 'create',
					'{"occurredAt":"2026-09-05T00:00:00Z","paymentMethod":"cash","paymentStatus":"pending","items":[],"subtotal":70000,"total":70000}'::jsonb,
					?, ?, ?, ?, ?, now()
				)`,
				args: []any{transactionID, userID, sessionID, terminalID, userID, sessionID},
			},
			{
				query: `INSERT INTO transaction_items (
					transaction_id, revision, line_number, package_id, package_revision,
					package_code, package_name, package_description, unit_price, quantity, line_total
				) VALUES (?, 1, 1, ?, 1, 'STANDARD', 'Paket Standar', '', 70000, 1, 70000)`,
				args: []any{transactionID, uuid.MustParse(standardPackageID)},
			},
			{
				query: `INSERT INTO print_attempts (
					id, transaction_id, transaction_revision, terminal_id, actor_id, session_id,
					status, printer_kind, client_occurred_at
				) VALUES (?, ?, 1, ?, ?, ?, 'success', 'simulator', now())`,
				args: []any{uuid.New(), transactionID, terminalID, userID, sessionID},
			},
			{
				query: `INSERT INTO audit_events (
					event_type, aggregate_type, aggregate_id, origin_actor_id, origin_session_id,
					submitted_by_actor_id, submitted_by_session_id, terminal_id, occurred_at
				) VALUES ('transaction.created', 'transaction', ?, ?, ?, ?, ?, ?, now())`,
				args: []any{transactionID, userID, sessionID, userID, sessionID, terminalID},
			},
			{
				query: `INSERT INTO sync_changes (
					aggregate, aggregate_id, action, revision, payload, tombstone
				) VALUES ('transaction', ?, 'created', 1, '{}'::jsonb, false)`,
				args: []any{transactionID},
			},
			{
				query: `INSERT INTO idempotency_records (
					terminal_id, operation_id, request_hash, response_status, response
				) VALUES (?, ?, ?, 201, '{}'::jsonb)`,
				args: []any{terminalID, uuid.NewString(), bytesOf(2, 32)},
			},
		}
		for _, statement := range statements {
			if statementErr := tx.Exec(statement.query, statement.args...).Error; statementErr != nil {
				return statementErr
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("insert pre-sandbox rows: %v", err)
	}

	if err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return migrateSandboxDataSpaces(ctx, tx)
	}); err != nil {
		t.Fatalf("apply Sandbox migration over append-only rows: %v", err)
	}

	for _, table := range []string{
		"sessions", "packages", "package_revisions", "transactions",
		"transaction_revisions", "transaction_items", "print_attempts",
		"audit_events", "sync_changes", "idempotency_records",
	} {
		var wrongSpaceRows int64
		if err = db.WithContext(ctx).Raw(
			"SELECT count(*) FROM "+table+" WHERE data_space_id <> ?",
			domain.LiveDataSpaceID(),
		).Scan(&wrongSpaceRows).Error; err != nil {
			t.Fatalf("check %s backfill: %v", table, err)
		}
		if wrongSpaceRows != 0 {
			t.Fatalf("%s has %d unexpected non-production backfilled rows", table, wrongSpaceRows)
		}
	}

	var revisionPaymentAmount *int64
	if err = db.WithContext(ctx).Raw(`
		SELECT payment_amount FROM transaction_revisions
		WHERE transaction_id = ? AND revision = 1`, transactionID,
	).Scan(&revisionPaymentAmount).Error; err != nil {
		t.Fatalf("read legacy revision payment amount: %v", err)
	}
	if revisionPaymentAmount != nil {
		t.Fatalf("append-only historical payment amount was rewritten: %v", *revisionPaymentAmount)
	}

	var paymentAmount int64
	if err = db.WithContext(ctx).Raw(
		`SELECT payment_amount FROM transactions WHERE id = ?`, transactionID,
	).Scan(&paymentAmount).Error; err != nil || paymentAmount != 70_000 {
		t.Fatalf("transaction payment amount=%d error=%v", paymentAmount, err)
	}
	var productionSpaceText string
	if err = db.WithContext(ctx).Raw(`
		SELECT data_space_id FROM sessions WHERE id = ?`, sessionID,
	).Scan(&productionSpaceText).Error; err != nil {
		t.Fatalf("read existing production session scope: %v", err)
	}
	if productionSpaceText != domain.LiveDataSpaceIDString {
		t.Fatalf("existing production session scope = %s, want %s", productionSpaceText, domain.LiveDataSpaceIDString)
	}
	compatibilitySessionID := uuid.New()
	if err = db.WithContext(ctx).Exec(`
		INSERT INTO sessions (id, user_id, terminal_id, token_hash)
		VALUES (?, ?, ?, ?)`,
		compatibilitySessionID, userID, terminalID, bytesOf(7, 32),
	).Error; err != nil {
		t.Fatalf("insert production session through pre-scope column contract: %v", err)
	}
	productionSpaceText = ""
	if err = db.WithContext(ctx).Raw(`
		SELECT data_space_id FROM sessions WHERE id = ?`, compatibilitySessionID,
	).Scan(&productionSpaceText).Error; err != nil {
		t.Fatalf("read compatibility session scope: %v", err)
	}
	if productionSpaceText != domain.LiveDataSpaceIDString {
		t.Fatalf("compatibility session scope = %s, want production", productionSpaceText)
	}
	assertLegacyProductionTransactionWritesRemainCompatible(
		t,
		ctx,
		db,
		userID,
		terminalID,
		compatibilitySessionID,
	)

	var activeSandboxCount int64
	if err = db.WithContext(ctx).Raw(`
		SELECT count(*) FROM data_spaces WHERE mode = 'sandbox' AND status = 'active'`,
	).Scan(&activeSandboxCount).Error; err != nil || activeSandboxCount != 0 {
		t.Fatalf("migration activated %d Sandbox generations; want schema/backfill only (error=%v)", activeSandboxCount, err)
	}
	sandboxID := uuid.New()
	if err = db.WithContext(ctx).Exec(`
		INSERT INTO data_spaces (id, mode, generation, status)
		VALUES (?, 'sandbox', 1, 'active')`, sandboxID,
	).Error; err != nil {
		t.Fatalf("create test-only Sandbox space for cross-space constraints: %v", err)
	}

	err = db.WithContext(ctx).Exec(`
		INSERT INTO package_revisions (
			package_id, revision, data_space_id, name, description,
			unit_price, change_reason, created_by
		) VALUES (?, 99, ?, 'Cross-space', '', 1000, 'uji constraint', ?)
	`, uuid.MustParse(standardPackageID), sandboxID, userID).Error
	assertPostgresConstraint(t, err, "package_revisions_package_space_fkey")

	err = db.WithContext(ctx).Exec(`
		INSERT INTO audit_events (
			data_space_id, event_type, aggregate_type, aggregate_id,
			origin_session_id, submitted_by_session_id, occurred_at
		) VALUES (?, 'cross-space.test', 'session', ?, ?, ?, now())
	`, sandboxID, sessionID.String(), sessionID, sessionID).Error
	assertPostgresConstraint(t, err, "audit_events_origin_session_space_fkey")

	err = db.WithContext(ctx).Exec(`
		INSERT INTO transaction_items (
			transaction_id, revision, line_number, data_space_id,
			package_id, package_revision, package_code, package_name,
			package_description, unit_price, quantity, line_total
		) VALUES (?, 1, 2, ?, ?, 1, 'STANDARD', 'Cross-space', '', 70000, 1, 70000)
	`, transactionID, sandboxID, uuid.MustParse(standardPackageID)).Error
	assertPostgresConstraint(t, err, "transaction_items_transaction_revision_space_fkey")
}

func assertLegacyProductionTransactionWritesRemainCompatible(
	t *testing.T,
	ctx context.Context,
	db *gorm.DB,
	userID uuid.UUID,
	terminalID uuid.UUID,
	sessionID uuid.UUID,
) {
	t.Helper()
	const transactionID = "01ARZ3NDEKTSV4RRFFQ69G5FAW"
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// This intentionally uses the pre-000004 write shape: neither the
		// data-space nor payment-amount columns are named by the old replica.
		if statementErr := tx.Exec(`
			INSERT INTO transactions (
				id, current_revision, occurred_at, origin_actor_id,
				origin_session_id, terminal_id, updated_by, subtotal, total,
				payment_method, payment_status
			) VALUES (?, 1, now(), ?, ?, ?, ?, 70000, 70000, 'cash', 'pending')`,
			transactionID, userID, sessionID, terminalID, userID,
		).Error; statementErr != nil {
			return statementErr
		}
		if statementErr := tx.Exec(`
			INSERT INTO transaction_revisions (
				transaction_id, revision, change_type, after_snapshot,
				origin_actor_id, origin_session_id, terminal_id,
				submitted_by_actor_id, submitted_by_session_id, client_occurred_at
			) VALUES (
				?, 1, 'create',
				'{"occurredAt":"2026-09-05T00:00:00Z","paymentMethod":"cash","paymentStatus":"pending","items":[],"subtotal":70000,"total":70000}'::jsonb,
				?, ?, ?, ?, ?, now()
			)`, transactionID, userID, sessionID, terminalID, userID, sessionID,
		).Error; statementErr != nil {
			return statementErr
		}
		if statementErr := tx.Exec(`
			INSERT INTO transaction_items (
				transaction_id, revision, line_number, package_id, package_revision,
				package_code, package_name, package_description, unit_price, quantity, line_total
			) VALUES (?, 1, 1, ?, 1, 'STANDARD', 'Paket Standar', '', 70000, 1, 70000)`,
			transactionID, uuid.MustParse(standardPackageID),
		).Error; statementErr != nil {
			return statementErr
		}
		return nil
	}); err != nil {
		t.Fatalf("legacy production create after Sandbox migration: %v", err)
	}

	var createdPaymentAmount int64
	var createdDataSpaceID string
	if err := db.WithContext(ctx).Raw(`
		SELECT payment_amount, data_space_id
		FROM transactions WHERE id = ?`, transactionID,
	).Row().Scan(&createdPaymentAmount, &createdDataSpaceID); err != nil {
		t.Fatalf("read legacy production create: %v", err)
	}
	if createdPaymentAmount != 70_000 || createdDataSpaceID != domain.LiveDataSpaceIDString {
		t.Fatalf("legacy production create payment=%d space=%s", createdPaymentAmount, createdDataSpaceID)
	}

	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if statementErr := tx.Exec(`
			INSERT INTO transaction_revisions (
				transaction_id, revision, base_revision, change_type, reason,
				before_snapshot, after_snapshot, origin_actor_id,
				origin_session_id, terminal_id, submitted_by_actor_id,
				submitted_by_session_id, client_occurred_at
			) VALUES (
				?, 2, 1, 'correction', 'Koreksi paket lama',
				'{"subtotal":70000,"total":70000}'::jsonb,
				'{"occurredAt":"2026-09-05T00:00:00Z","paymentMethod":"cash","paymentStatus":"pending","items":[],"subtotal":100000,"total":100000}'::jsonb,
				?, ?, ?, ?, ?, now()
			)`, transactionID, userID, sessionID, terminalID, userID, sessionID,
		).Error; statementErr != nil {
			return statementErr
		}
		if statementErr := tx.Exec(`
			INSERT INTO transaction_items (
				transaction_id, revision, line_number, package_id, package_revision,
				package_code, package_name, package_description, unit_price, quantity, line_total
			) VALUES (?, 2, 1, ?, 1, 'SUNRISE', 'Paket Sunrise', '', 100000, 1, 100000)`,
			transactionID, uuid.MustParse(sunrisePackageID),
		).Error; statementErr != nil {
			return statementErr
		}
		// The old correction statement changes total but cannot name the new
		// payment_amount column. The compatibility trigger must follow total.
		return tx.Exec(`
			UPDATE transactions
			SET current_revision = 2, subtotal = 100000, total = 100000,
			    updated_by = ?, updated_at = now()
			WHERE id = ?`, userID, transactionID,
		).Error
	}); err != nil {
		t.Fatalf("legacy production correction after Sandbox migration: %v", err)
	}

	var correctedPaymentAmount int64
	if err := db.WithContext(ctx).Raw(`
		SELECT payment_amount FROM transactions WHERE id = ?`, transactionID,
	).Scan(&correctedPaymentAmount).Error; err != nil {
		t.Fatalf("read legacy production correction: %v", err)
	}
	if correctedPaymentAmount != 100_000 {
		t.Fatalf("legacy production correction payment=%d, want 100000", correctedPaymentAmount)
	}
}

func sandboxScopedDatabaseURL(t *testing.T, databaseURL, schemaName string) string {
	t.Helper()
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse DATABASE_URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schemaName)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func bytesOf(value byte, count int) []byte {
	result := make([]byte, count)
	for index := range result {
		result[index] = value
	}
	return result
}

func assertPostgresConstraint(t *testing.T, err error, name string) {
	t.Helper()
	if err == nil {
		t.Fatalf("constraint %s accepted a cross-space reference", name)
	}
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) ||
		postgresError.Code != "23503" ||
		postgresError.ConstraintName != name {
		t.Fatalf("cross-space error=%v, want 23503 from %s", err, name)
	}
}
