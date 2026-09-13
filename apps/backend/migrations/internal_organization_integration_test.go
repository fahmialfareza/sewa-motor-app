package migrations

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestInternalOrganizationMigrationPreservesEvidenceAndPublishesGlobalRoles(t *testing.T) {
	const productionDataSpaceID = "00000000-0000-4000-8000-000000000100"
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not configured")
	}
	base, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	schema := "organization_migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = base.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		base.Exec(`DROP SCHEMA "` + schema + `" CASCADE`)
		if sql, e := base.DB(); e == nil {
			sql.Close()
		}
	})
	db, err := gorm.Open(postgres.Open(sandboxScopedDatabaseURL(t, databaseURL, schema)), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sql, e := db.DB(); e == nil {
			sql.Close()
		}
	})
	ctx := context.Background()
	for _, m := range orderedMigrations[:5] {
		if err = db.Transaction(func(tx *gorm.DB) error { return m.up(ctx, tx) }); err != nil {
			t.Fatal(err)
		}
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if err := db.Exec(query, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	memberSuper, platformSuper, cashier, disabled, deleted := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	accounts := []struct {
		id                                     uuid.UUID
		name, membershipRole, membershipStatus string
		active, platform, deleted              bool
		want                                   string
	}{
		{memberSuper, "member-super", "superadmin", "active", true, false, false, "superadmin"},
		{platformSuper, "platform-super", "admin", "inactive", true, true, false, "superadmin"},
		{cashier, "cashier", "admin", "active", true, false, false, "admin"},
		{disabled, "disabled", "superadmin", "active", false, true, false, "admin"},
		{deleted, "deleted", "superadmin", "active", false, true, true, "admin"},
	}
	for _, account := range accounts {
		exec(`INSERT INTO users(id,full_name,username,password_hash,role,is_active,must_change_password,is_platform_admin,deleted_at)
		 VALUES (?,?,?,'unchanged-credential','admin',?,false,?,CASE WHEN ? THEN now() ELSE NULL END)`,
			account.id, account.name, account.name, account.active, account.platform, account.deleted)
		exec(`INSERT INTO tenant_memberships(tenant_id,user_id,role,status) VALUES (?,?,?,?)`, initialTenantID, account.id, account.membershipRole, account.membershipStatus)
	}
	legacySession, invitation, oldEvent := uuid.New(), uuid.New(), uuid.New()
	exec(`INSERT INTO sessions(id,user_id,token_hash,context_kind,tenant_id,membership_id,data_space_id,legacy_origin)
	 SELECT ?,?,?, 'tenant',tenant_id,id,?,true FROM tenant_memberships WHERE user_id=?`,
		legacySession, memberSuper, bytesOf(44, 32), productionDataSpaceID, memberSuper)
	exec(`INSERT INTO tenant_invitations(id,tenant_id,role,code_hash,expires_at,created_by)
	 VALUES (?,?,'admin',?,now()+interval '7 days',?)`, invitation, initialTenantID, bytesOf(45, 32), memberSuper)
	exec(`INSERT INTO sync_changes(data_space_id,tenant_id,aggregate,aggregate_id,action,payload)
	 VALUES (?,?,'user',?,'updated','{"signedEvidence":"must-remain-byte-identical"}')`, productionDataSpaceID, initialTenantID, oldEvent.String())
	var oldSession, oldMemberships, oldPackages, oldChanges string
	for _, item := range []struct {
		query  string
		target *string
	}{
		{`SELECT to_jsonb(s)::text FROM sessions s WHERE id='` + legacySession.String() + `'`, &oldSession},
		{`SELECT jsonb_agg(to_jsonb(m) ORDER BY id)::text FROM tenant_memberships m`, &oldMemberships},
		{`SELECT jsonb_agg(to_jsonb(r) ORDER BY package_id,revision)::text FROM package_revisions r`, &oldPackages},
		{`SELECT jsonb_agg(to_jsonb(c) ORDER BY cursor)::text FROM sync_changes c`, &oldChanges},
	} {
		if err := db.Raw(item.query).Scan(item.target).Error; err != nil {
			t.Fatal(err)
		}
	}
	var oldCursor int64
	if err := db.Raw(`SELECT COALESCE(max(cursor),0) FROM sync_changes`).Scan(&oldCursor).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(func(tx *gorm.DB) error { return migrateInternalOrganization(ctx, tx) }); err != nil {
		t.Fatal(err)
	}
	for _, account := range accounts {
		var role string
		var preserved bool
		if err := db.Raw(`SELECT role FROM users WHERE id=?`, account.id).Scan(&role).Error; err != nil {
			t.Fatal(err)
		}
		if role != account.want {
			t.Fatalf("%s role=%s want %s", account.name, role, account.want)
		}
		db.Raw(`SELECT is_active=? AND (deleted_at IS NOT NULL)=? AND password_hash='unchanged-credential' FROM users WHERE id=?`, account.active, account.deleted, account.id).Scan(&preserved)
		if !preserved {
			t.Fatalf("account lifecycle/credential changed for %s", account.name)
		}
	}
	checks := []struct {
		name, query string
		args        []any
	}{
		{"session evidence", `SELECT (to_jsonb(s)-'protocol_version'-'sandbox_qris_policy')=?::jsonb AND protocol_version=2 AND sandbox_qris_policy='fixed_1000' FROM sessions s WHERE id=?`, []any{oldSession, legacySession}},
		{"membership history", `SELECT jsonb_agg(to_jsonb(m) ORDER BY id)=?::jsonb FROM tenant_memberships m`, []any{oldMemberships}},
		{"append-only revisions", `SELECT jsonb_agg(to_jsonb(r) ORDER BY package_id,revision)=?::jsonb FROM package_revisions r`, []any{oldPackages}},
		{"old cursors/events", `SELECT jsonb_agg(to_jsonb(c) ORDER BY cursor)=?::jsonb FROM sync_changes c WHERE cursor<=?`, []any{oldChanges, oldCursor}},
		{"invitation retired", `SELECT revoked_at IS NOT NULL AND accepted_at IS NULL FROM tenant_invitations WHERE id=?`, []any{invitation}},
		{"safe role refresh", `SELECT count(*)=5 AND bool_and(NOT jsonb_exists(payload,'passwordHash') AND NOT jsonb_exists(payload,'password_hash')) FROM sync_changes WHERE data_space_id=? AND aggregate='user' AND cursor>?`, []any{productionDataSpaceID, oldCursor}},
		{"metadata refresh", `SELECT count(*)=1 FROM sync_changes WHERE data_space_id=? AND aggregate='tenant_metadata' AND payload->>'name'='Telomoyo' AND cursor>?`, []any{productionDataSpaceID, oldCursor}},
	}
	for _, check := range checks {
		var ok bool
		if err := db.Raw(check.query, check.args...).Scan(&ok).Error; err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("migration changed %s", check.name)
		}
	}
	if err := db.Exec(`UPDATE sessions SET protocol_version=3,sandbox_qris_policy='transaction_total' WHERE id=?`, legacySession).Error; err == nil {
		t.Fatal("session policy changed in place")
	}
	if err := db.Exec(`UPDATE package_revisions SET name='rewritten'`).Error; err == nil {
		t.Fatal("append-only protection removed")
	}
}
