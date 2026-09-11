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

func TestTenantsMigrationPreservesLegacyAndRejectsCrossTenantReferences(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not configured")
	}
	base, err := gorm.Open(postgres.Open(databaseURL), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	schema := "tenants_migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = base.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		base.Exec(`DROP SCHEMA "` + schema + `" CASCADE`)
		if db, e := base.DB(); e == nil {
			db.Close()
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
	for _, m := range orderedMigrations[:4] {
		if err = db.Transaction(func(tx *gorm.DB) error { return m.up(ctx, tx) }); err != nil {
			t.Fatal(err)
		}
	}
	actor, deleted, terminal, session := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO users(id,full_name,username,password_hash,role,is_active,must_change_password) VALUES (?,'First','first','hash','superadmin',true,false)`, []any{actor}},
		{`INSERT INTO users(id,full_name,username,password_hash,role,is_active,deleted_at) VALUES (?,'Deleted','deleted','hash','admin',false,now())`, []any{deleted}},
		{`INSERT INTO terminals(id,installation_id,name,public_key,enrolled_by) VALUES (?,?,'MPOS',?,?)`, []any{terminal, uuid.NewString(), make([]byte, 32), actor}},
		{`INSERT INTO sessions(id,user_id,terminal_id,token_hash) VALUES (?,?,?,?)`, []any{session, actor, terminal, bytesOf(7, 32)}},
	} {
		if err = db.Exec(q.sql, q.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	var oldRevision string
	if err = db.Raw(`SELECT row_to_json(r)::text FROM package_revisions r ORDER BY package_id LIMIT 1`).Scan(&oldRevision).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(func(tx *gorm.DB) error { return migrateTenants(ctx, tx) }); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.Raw(`SELECT count(*) FROM tenant_memberships WHERE tenant_id=?`, initialTenantID).Scan(&count)
	if count != 2 {
		t.Fatalf("historical members=%d, want 2", count)
	}
	var active string
	db.Raw(`SELECT status FROM tenant_memberships WHERE user_id=?`, deleted).Scan(&active)
	if active != "inactive" {
		t.Fatalf("deleted account membership=%s", active)
	}
	var preserved bool
	db.Raw(`SELECT id=? AND tenant_id=? AND context_kind='tenant' AND legacy_origin AND revoked_at IS NULL FROM sessions WHERE id=?`, session, initialTenantID, session).Scan(&preserved)
	if !preserved {
		t.Fatal("legacy session changed")
	}
	var revisionUnchanged bool
	db.Raw(`SELECT (to_jsonb(r)-'tenant_id')=?::jsonb FROM package_revisions r ORDER BY package_id LIMIT 1`, oldRevision).Scan(&revisionUnchanged)
	if !revisionUnchanged {
		t.Fatal("append-only revision was rewritten")
	}
	if err = db.Exec(`UPDATE package_revisions SET name='changed'`).Error; err == nil {
		t.Fatal("append-only update accepted")
	}
	otherTenant, otherSpace := uuid.New(), uuid.New()
	if err = db.Transaction(func(tx *gorm.DB) error {
		queries := []struct {
			q    string
			args []any
		}{
			{`INSERT INTO tenants(id,name,slug,status) VALUES (?,'Other','other','active')`, []any{otherTenant}},
			{`INSERT INTO tenant_profile_revisions(tenant_id,revision,business_name) VALUES (?,1,'Other')`, []any{otherTenant}},
			{`INSERT INTO tenant_memberships(tenant_id,user_id,role) VALUES (?,?,'admin')`, []any{otherTenant, actor}},
			{`INSERT INTO data_spaces(id,tenant_id,mode,generation,status) VALUES (?,?,'production',1,'active')`, []any{otherSpace, otherTenant}},
		}
		for _, q := range queries {
			if e := tx.Exec(q.q, q.args...).Error; e != nil {
				return e
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`INSERT INTO sessions(user_id,terminal_id,token_hash,data_space_id) VALUES (?,?,?,?)`, actor, terminal, bytesOf(8, 32), otherSpace).Error; err == nil {
		t.Fatal("cross-tenant terminal session accepted")
	}
	if err = db.Exec(`UPDATE sessions SET data_space_id=? WHERE id=?`, otherSpace, session).Error; err == nil {
		t.Fatal("immutable session changed tenants")
	}
	if err = db.Exec(`INSERT INTO sessions(context_kind,user_id,token_hash) VALUES ('account',?,?)`, actor, bytesOf(9, 32)).Error; err != nil {
		t.Fatalf("account session: %v", err)
	}
	if err = db.Exec(`INSERT INTO sessions(context_kind,user_id,token_hash,data_space_id) VALUES ('platform',?,?,?)`, actor, bytesOf(10, 32), otherSpace).Error; err == nil {
		t.Fatal("platform session acquired business scope")
	}
	if err = db.Exec(`INSERT INTO sync_changes(data_space_id,tenant_id,aggregate,aggregate_id,action,payload) VALUES (?,?,'user',?,'updated','{}')`, otherSpace, initialTenantID, actor.String()).Error; err == nil {
		t.Fatal("cross-tenant sync event accepted")
	}
	if err = db.Exec(`INSERT INTO sync_changes(aggregate,aggregate_id,action,payload) VALUES ('user',?,'updated','{}')`, actor.String()).Error; err == nil {
		t.Fatal("missing data scope accepted")
	}
}
