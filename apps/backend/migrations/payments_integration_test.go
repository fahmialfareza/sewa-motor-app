package migrations

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPaymentMigrationBackfillsLegacyTransactions(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not configured")
	}

	base, err := gorm.Open(gormpostgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	schemaName := "payment_migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := `"` + schemaName + `"`
	if err := base.Exec("CREATE SCHEMA " + quotedSchema).Error; err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	t.Cleanup(func() {
		if dropErr := base.Exec("DROP SCHEMA " + quotedSchema + " CASCADE").Error; dropErr != nil {
			t.Errorf("drop test schema: %v", dropErr)
		}
	})

	scopedURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse DATABASE_URL: %v", err)
	}
	query := scopedURL.Query()
	query.Set("search_path", schemaName)
	scopedURL.RawQuery = query.Encode()
	db, err := gorm.Open(gormpostgres.Open(scopedURL.String()), &gorm.Config{})
	if err != nil {
		t.Fatalf("open scoped integration database: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return migrateInitialSchema(ctx, tx)
	}); err != nil {
		t.Fatalf("apply initial migration: %v", err)
	}

	userID := uuid.New()
	terminalID := uuid.New()
	sessionID := uuid.New()
	transactionID := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		statements := []struct {
			query string
			args  []any
		}{
			{
				query: `INSERT INTO users (
					id, full_name, username, password_hash, role, is_active, must_change_password
				) VALUES (?, 'Legacy Admin', ?, 'hash', 'admin', true, false)`,
				args: []any{userID, "legacy_admin"},
			},
			{
				query: `INSERT INTO terminals (
					id, installation_id, name, public_key, enrolled_by
				) VALUES (?, ?, 'Legacy Terminal', ?, ?)`,
				args: []any{terminalID, uuid.NewString(), make([]byte, 32), userID},
			},
			{
				query: `INSERT INTO sessions (
					id, user_id, terminal_id, token_hash
				) VALUES (?, ?, ?, ?)`,
				args: []any{sessionID, userID, terminalID, make([]byte, 32)},
			},
			{
				query: `INSERT INTO transactions (
					id, current_revision, occurred_at, origin_actor_id,
					origin_session_id, terminal_id, updated_by, subtotal, total
				) VALUES (?, 1, now(), ?, ?, ?, ?, 70000, 70000)`,
				args: []any{transactionID, userID, sessionID, terminalID, userID},
			},
			{
				query: `INSERT INTO transaction_revisions (
					transaction_id, revision, change_type, after_snapshot,
					origin_actor_id, origin_session_id, terminal_id,
					submitted_by_actor_id, submitted_by_session_id, client_occurred_at
				) VALUES (
					?, 1, 'create',
					'{"occurredAt":"2026-07-29T00:00:00Z","items":[],"subtotal":70000,"total":70000}'::jsonb,
					?, ?, ?, ?, ?, now()
				)`,
				args: []any{transactionID, userID, sessionID, terminalID, userID, sessionID},
			},
		}
		for _, statement := range statements {
			if err := tx.Exec(statement.query, statement.args...).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("insert legacy transaction: %v", err)
	}

	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return migrateTransactionPayments(ctx, tx)
	}); err != nil {
		t.Fatalf("apply payment migration: %v", err)
	}

	var result struct {
		Method    string
		Status    string
		Confirmed *int
		Revision  int
	}
	if err := db.WithContext(ctx).Raw(`
		SELECT payment_method AS method, payment_status AS status,
		       payment_confirmed_revision AS confirmed, current_revision AS revision
		FROM transactions WHERE id = ?`,
		transactionID,
	).Scan(&result).Error; err != nil {
		t.Fatalf("read migrated transaction: %v", err)
	}
	if result.Method != "legacy" ||
		result.Status != "success" ||
		result.Confirmed == nil ||
		*result.Confirmed != result.Revision {
		t.Fatalf("unexpected payment backfill: %+v", result)
	}

	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Exec(`
			UPDATE transactions
			SET payment_status = 'failed'
			WHERE id = ?`,
			transactionID,
		).Error
	})
	if err == nil {
		t.Fatal("payment confirmation shape constraint accepted an invalid state")
	}

	if !db.Migrator().HasIndex(
		&transactionPaymentModel{},
		"transactions_paid_occurred_idx",
	) {
		t.Fatal("successful payment reporting index was not created")
	}

	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return migrateQrisPayloadBinding(ctx, tx)
	}); err != nil {
		t.Fatalf("apply QRIS payload binding migration: %v", err)
	}
	if !db.Migrator().HasColumn(
		&qrisPayloadBindingTransactionModel{},
		"QrisPayloadHash",
	) || !db.Migrator().HasColumn(
		&qrisPayloadBindingRevisionModel{},
		"QrisPayloadHash",
	) {
		t.Fatal("QRIS payload binding columns were not created")
	}
	var historicalRevision struct {
		QrisPayloadHash *string
	}
	if err := db.WithContext(ctx).Raw(`
		SELECT qris_payload_hash
		FROM transaction_revisions
		WHERE transaction_id = ? AND revision = 1`,
		transactionID,
	).Scan(&historicalRevision).Error; err != nil {
		t.Fatalf("read historical revision QRIS binding: %v", err)
	}
	if historicalRevision.QrisPayloadHash != nil {
		t.Fatalf(
			"historical revision QRIS payload hash = %q, want NULL",
			*historicalRevision.QrisPayloadHash,
		)
	}

	validHash := strings.Repeat("ab", 32)
	if err := db.WithContext(ctx).Exec(`
		UPDATE transactions
		SET qris_payload_hash = ?
		WHERE id = ?`,
		validHash,
		transactionID,
	).Error; err == nil {
		t.Fatal("legacy transaction accepted a QRIS payload hash")
	}

	// Existing QRIS rows cannot be truthfully backfilled and therefore remain
	// nullable. Application validation requires the hash on every new write.
	if err := db.WithContext(ctx).Exec(`
		UPDATE transactions
		SET payment_method = 'qris', qris_payload_hash = NULL
		WHERE id = ?`,
		transactionID,
	).Error; err != nil {
		t.Fatalf("pre-binding QRIS transaction was rejected: %v", err)
	}
	if err := db.WithContext(ctx).Exec(`
		UPDATE transactions
		SET qris_payload_hash = ?
		WHERE id = ?`,
		validHash,
		transactionID,
	).Error; err != nil {
		t.Fatalf("valid QRIS payload hash was rejected: %v", err)
	}
	if err := db.WithContext(ctx).Exec(`
		UPDATE transactions
		SET qris_payload_hash = ?
		WHERE id = ?`,
		strings.ToUpper(validHash),
		transactionID,
	).Error; err == nil {
		t.Fatal("uppercase QRIS payload hash was accepted")
	}
	err = db.WithContext(ctx).Exec(`
		UPDATE transaction_revisions
		SET qris_payload_hash = ?
		WHERE transaction_id = ? AND revision = 1`,
		validHash,
		transactionID,
	).Error
	if err == nil {
		t.Fatal("legacy revision snapshot accepted a QRIS payload hash")
	}
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != "55000" {
		t.Fatalf("append-only revision update error = %v, want SQLSTATE 55000", err)
	}
}
