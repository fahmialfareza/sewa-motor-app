package main

import (
	"context"
	"crypto/sha256"
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

func TestAccountAdminGlobalRoleAndRecoveryPreserveHistory(t *testing.T) {
	store := operatorTestStore(t)
	ctx := context.Background()
	account := uuid.New()
	if _, err := store.Pool.Exec(ctx, `INSERT INTO users(id,username,full_name,password_hash,role,must_change_password) VALUES($1,'operator-target','Target','old-hash','admin',false)`, account); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool.Exec(ctx, `INSERT INTO tenant_memberships(tenant_id,user_id,role) VALUES($1,$2,'admin')`, domain.InitialTenantID(), account); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool.Exec(ctx, `INSERT INTO users(username,full_name,password_hash,role) VALUES('remaining-superadmin','Other Superadmin','old-hash','superadmin')`); err != nil {
		t.Fatal(err)
	}
	secondTenant := uuid.New()
	tenantTx, err := store.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tenantTx.Rollback(ctx)
	if _, err := tenantTx.Exec(ctx, `INSERT INTO tenants(id,name,slug,status) VALUES($1,'Second','second','active')`, secondTenant); err != nil {
		t.Fatal(err)
	}
	if _, err := tenantTx.Exec(ctx, `INSERT INTO tenant_profile_revisions(tenant_id,revision,business_name) VALUES($1,1,'Second')`, secondTenant); err != nil {
		t.Fatal(err)
	}
	if _, err := tenantTx.Exec(ctx, `INSERT INTO data_spaces(id,tenant_id,mode,generation,status) VALUES($1,$2,'production',1,'active')`, uuid.New(), secondTenant); err != nil {
		t.Fatal(err)
	}
	if err := tenantTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	seedOperatorSessions(t, store, account, "before-grant")
	var before string
	if err := store.Pool.QueryRow(ctx, `SELECT row_to_json(m)::text FROM tenant_memberships m WHERE user_id=$1`, account).Scan(&before); err != nil {
		t.Fatal(err)
	}
	opts := options{username: "operator-target", action: "grant-superadmin", operator: "approved-operator", reason: "Approved global Superadmin"}
	if err := apply(ctx, store, opts, ""); err != nil {
		t.Fatal(err)
	}
	var isPlatform bool
	var role string
	if err := store.Pool.QueryRow(ctx, `SELECT role,is_platform_admin FROM users WHERE id=$1`, account).Scan(&role, &isPlatform); err != nil || role != "superadmin" || isPlatform {
		t.Fatalf("global role=%s independent platform=%v err=%v", role, isPlatform, err)
	}
	assertOperatorRevoked(t, store, account, 3, "account_access_changed")
	seedOperatorSessions(t, store, account, "after-grant")
	if err := apply(ctx, store, opts, ""); err != nil {
		t.Fatal(err)
	}
	var live int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE user_id=$1 AND revoked_at IS NULL`, account).Scan(&live); err != nil || live != 3 {
		t.Fatalf("no-op grant revoked sessions: %d %v", live, err)
	}
	opts.action = "revoke-superadmin"
	if err := apply(ctx, store, opts, ""); err != nil {
		t.Fatal(err)
	}
	if err := store.Pool.QueryRow(ctx, `SELECT role FROM users WHERE id=$1`, account).Scan(&role); err != nil || role != "admin" {
		t.Fatalf("global demotion role=%s err=%v", role, err)
	}
	assertOperatorRevoked(t, store, account, 6, "account_access_changed")
	seedOperatorSessions(t, store, account, "after-revoke")
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
	assertOperatorRevoked(t, store, account, 3, "password_recovered")
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
	if audit["operator"] != opts.operator || audit["reason"] != opts.reason || audit["revokedSessions"] != float64(3) || audit["role"] != "admin" || strings.Contains(string(metadata), hash) {
		t.Fatalf("audit missing attribution, count, or exposed hash: %s", metadata)
	}
	if _, err := store.Pool.Exec(ctx, `UPDATE platform_audit_events SET event_type='rewritten'`); err == nil {
		t.Fatal("operator audit history is mutable")
	}
	var projections, leaked int
	if err := store.Pool.QueryRow(ctx, `SELECT count(DISTINCT tenant_id),count(*) FILTER(WHERE payload::text LIKE '%' || $2::text || '%' OR payload ? 'passwordHash' OR payload ? 'password_hash') FROM sync_changes WHERE aggregate='user' AND aggregate_id=$1`, account.String(), hash).Scan(&projections, &leaked); err != nil || projections != 2 || leaked != 0 {
		t.Fatalf("safe projections tenants=%d leaked=%d err=%v", projections, leaked, err)
	}
}

func TestAccountAdminConcurrentDemotionProtectsLastSuperadmin(t *testing.T) {
	store := operatorTestStore(t)
	ctx := context.Background()
	for _, name := range []string{"first", "second"} {
		if _, err := store.Pool.Exec(ctx, `INSERT INTO users(username,full_name,password_hash,role) VALUES($1,$1,'test-hash','superadmin')`, name); err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, name := range []string{"first", "second"} {
		go func(name string) {
			<-start
			results <- apply(ctx, store, options{username: name, action: "revoke-superadmin", operator: "operator", reason: "Concurrent authorized role changes"}, "")
		}(name)
	}
	close(start)
	succeeded, blocked := 0, 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			succeeded++
		} else if strings.Contains(err.Error(), "last active Superadmin") {
			blocked++
		} else {
			t.Fatal(err)
		}
	}
	var remaining int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE role='superadmin' AND is_active AND deleted_at IS NULL`).Scan(&remaining); err != nil || remaining != 1 || succeeded != 1 || blocked != 1 {
		t.Fatalf("remaining=%d succeeded=%d blocked=%d err=%v", remaining, succeeded, blocked, err)
	}
}

func TestAccountAdminDoesNotReactivateOrMutateDisabledAccounts(t *testing.T) {
	store := operatorTestStore(t)
	ctx := context.Background()
	if _, err := store.Pool.Exec(ctx, `INSERT INTO users(username,full_name,password_hash,role,is_active) VALUES('disabled','Disabled','unchanged-hash','admin',false)`); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"grant-superadmin", "revoke-superadmin", "reset-password"} {
		if err := apply(ctx, store, options{username: "disabled", action: action, operator: "operator", reason: "Test"}, "replacement-hash"); err == nil || !strings.Contains(err.Error(), "inactive or deleted") {
			t.Fatalf("disabled account operation=%s err=%v", action, err)
		}
	}
}

func seedOperatorSessions(t *testing.T, store *postgres.Store, account uuid.UUID, prefix string) {
	t.Helper()
	for _, kind := range []string{"account", "platform", "tenant"} {
		var space any
		if kind == "tenant" {
			space = domain.LiveDataSpaceID()
		}
		hash := sha256.Sum256([]byte(prefix + kind))
		if _, err := store.Pool.Exec(context.Background(), `INSERT INTO sessions(user_id,context_kind,data_space_id,token_hash) VALUES($1,$2,$3,$4)`, account, kind, space, hash[:]); err != nil {
			t.Fatal(err)
		}
	}
}

func assertOperatorRevoked(t *testing.T, store *postgres.Store, account uuid.UUID, expected int, reason string) {
	t.Helper()
	var live, revoked int
	if err := store.Pool.QueryRow(context.Background(), `SELECT count(*) FILTER(WHERE revoked_at IS NULL),count(*) FILTER(WHERE revoked_at IS NOT NULL AND revoked_reason=$2) FROM sessions WHERE user_id=$1`, account, reason).Scan(&live, &revoked); err != nil || live != 0 || revoked != expected {
		t.Fatalf("session revocation live=%d revoked=%d expected=%d reason=%s err=%v", live, revoked, expected, reason, err)
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
