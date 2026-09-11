package main

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/adapter/postgres"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/migrations"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAccountAdminRecoveryRevokesAllSessionsWithoutChangingMemberships(t *testing.T) {
	store := operatorTestStore(t)
	ctx := context.Background()
	account := uuid.New()
	if _, err := store.Pool.Exec(ctx, `INSERT INTO users(id,username,full_name,password_hash,role,must_change_password) VALUES($1,'operator-target','Target','old-hash','superadmin',false)`, account); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool.Exec(ctx, `INSERT INTO tenant_memberships(tenant_id,user_id,role) VALUES($1,$2,'admin')`, domain.InitialTenantID(), account); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"account", "platform", "tenant"} {
		var space any
		if kind == "tenant" {
			space = domain.LiveDataSpaceID()
		}
		if _, err := store.Pool.Exec(ctx, `INSERT INTO sessions(user_id,context_kind,data_space_id,token_hash) VALUES($1,$2,$3,$4)`, account, kind, space, []byte(strings.Repeat(kind[:1], 32))); err != nil {
			t.Fatal(err)
		}
	}
	var before string
	if err := store.Pool.QueryRow(ctx, `SELECT row_to_json(m)::text FROM tenant_memberships m WHERE user_id=$1`, account).Scan(&before); err != nil {
		t.Fatal(err)
	}
	opts := options{username: "operator-target", action: "grant-platform", operator: "approved-operator", reason: "Initial platform assignment"}
	if err := apply(ctx, store, opts, ""); err != nil {
		t.Fatal(err)
	}
	var isPlatform bool
	if err := store.Pool.QueryRow(ctx, `SELECT is_platform_admin FROM users WHERE id=$1`, account).Scan(&isPlatform); err != nil || !isPlatform {
		t.Fatalf("platform grant=%v err=%v", isPlatform, err)
	}
	var live int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE user_id=$1 AND revoked_at IS NULL`, account).Scan(&live); err != nil || live != 3 {
		t.Fatalf("grant unexpectedly revoked sessions: %d %v", live, err)
	}
	opts.action = "revoke-platform"
	if err := apply(ctx, store, opts, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE user_id=$1 AND revoked_at IS NULL`, account).Scan(&live); err != nil || live != 2 {
		t.Fatalf("platform revoke should preserve account/tenant sessions: %d %v", live, err)
	}
	opts.action = "reset-password"
	opts.reason = "Confirmed account-owner recovery"
	const hash = "test-only-replacement-argon2-hash"
	if err := apply(ctx, store, opts, hash); err != nil {
		t.Fatal(err)
	}
	var changed, mustChange bool
	if err := store.Pool.QueryRow(ctx, `SELECT password_hash=$2,must_change_password FROM users WHERE id=$1`, account, hash).Scan(&changed, &mustChange); err != nil || !changed || !mustChange {
		t.Fatalf("password reset=%v forced=%v err=%v", changed, mustChange, err)
	}
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE user_id=$1 AND revoked_at IS NULL`, account).Scan(&live); err != nil || live != 0 {
		t.Fatalf("password reset left active sessions: %d %v", live, err)
	}
	var after string
	if err := store.Pool.QueryRow(ctx, `SELECT row_to_json(m)::text FROM tenant_memberships m WHERE user_id=$1`, account).Scan(&after); err != nil || after != before {
		t.Fatalf("operator changed tenant membership: %v", err)
	}
	var metadata []byte
	if err := store.Pool.QueryRow(ctx, `SELECT metadata FROM platform_audit_events WHERE event_type='operator.reset-password'`).Scan(&metadata); err != nil {
		t.Fatal(err)
	}
	var audit map[string]any
	if err := json.Unmarshal(metadata, &audit); err != nil {
		t.Fatal(err)
	}
	if audit["operator"] != opts.operator || audit["reason"] != opts.reason || audit["revokedSessions"] != float64(2) || strings.Contains(string(metadata), hash) {
		t.Fatalf("audit missing attribution, count, or exposed hash: %s", metadata)
	}
	if _, err := store.Pool.Exec(ctx, `UPDATE platform_audit_events SET event_type='rewritten'`); err == nil {
		t.Fatal("operator audit history is mutable")
	}
}

// Only randomly named disposable schemas are created and dropped. No migration
// or operator mutation runs in the configured database's application schema.
func operatorTestStore(t *testing.T) *postgres.Store {
	t.Helper()
	raw := os.Getenv("DATABASE_URL")
	if raw == "" {
		t.Skip("DATABASE_URL is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	base, err := pgxpool.New(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	schema := "operator_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = base.Exec(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		base.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { base.Exec(context.Background(), `DROP SCHEMA "`+schema+`" CASCADE`); base.Close() })
	dsn, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	query := dsn.Query()
	query.Set("search_path", schema)
	dsn.RawQuery = query.Encode()
	db, err := gorm.Open(gormpostgres.Open(dsn.String()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sql, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sql.Close() })
	if err = migrations.Apply(ctx, db); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, dsn.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return &postgres.Store{Pool: pool}
}
