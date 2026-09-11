package migrations

import (
	"context"
	"fmt"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"gorm.io/gorm"
)

const initialTenantID = "00000000-0000-4000-8000-000000000200"

// migrateTenants preserves historical append-only rows with constant DDL
// defaults. New business rows derive tenant ownership from their explicit data
// space, never from a request-supplied tenant or a global fallback.
func migrateTenants(ctx context.Context, tx *gorm.DB) error {
	defer observability.StartSegment(ctx, "Migrations.Tenants")()
	statements := []string{
		`SET CONSTRAINTS ALL IMMEDIATE`,
		`SET CONSTRAINTS ALL DEFERRED`,
		`CREATE TABLE tenants (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 160),
			slug text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,62}$'),
			status text NOT NULL CHECK (status IN ('pending_setup','active','suspended')),
			profile_revision integer NOT NULL DEFAULT 1,
			qris_revision integer,
			created_by uuid REFERENCES users(id),
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
		fmt.Sprintf(`INSERT INTO tenants(id,name,slug,status) VALUES ('%s','Telomoyo','telomoyo','active')`, initialTenantID),
		`ALTER TABLE users ADD COLUMN is_platform_admin boolean NOT NULL DEFAULT false`,
		`CREATE TABLE tenant_memberships (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id uuid NOT NULL REFERENCES tenants(id),
			user_id uuid NOT NULL REFERENCES users(id),
			role text NOT NULL CHECK (role IN ('admin','superadmin')),
			status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','inactive')),
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			UNIQUE(tenant_id,user_id), UNIQUE(id,tenant_id,user_id)
		)`,
		fmt.Sprintf(`INSERT INTO tenant_memberships(tenant_id,user_id,role,status,created_at,updated_at)
			SELECT '%s',id,role,CASE WHEN is_active AND deleted_at IS NULL THEN 'active' ELSE 'inactive' END,created_at,updated_at FROM users`, initialTenantID),
		fmt.Sprintf(`ALTER TABLE data_spaces ADD COLUMN tenant_id uuid NOT NULL DEFAULT '%s' REFERENCES tenants(id)`, initialTenantID),
		`ALTER TABLE data_spaces ADD CONSTRAINT data_spaces_tenant_id_key UNIQUE(tenant_id,id)`,
		`ALTER TABLE data_spaces DROP CONSTRAINT data_spaces_mode_generation_key`,
		`DROP INDEX data_spaces_active_mode_key`,
		`ALTER TABLE data_spaces ADD CONSTRAINT data_spaces_tenant_mode_generation_key UNIQUE(tenant_id,mode,generation)`,
		`CREATE UNIQUE INDEX data_spaces_active_tenant_mode_key ON data_spaces(tenant_id,mode) WHERE status='active'`,
		`ALTER TABLE data_spaces ALTER COLUMN tenant_id DROP DEFAULT`,
		fmt.Sprintf(`ALTER TABLE terminals ADD COLUMN tenant_id uuid NOT NULL DEFAULT '%s' REFERENCES tenants(id)`, initialTenantID),
		`DROP INDEX terminals_installation_id_key`,
		`CREATE UNIQUE INDEX terminals_tenant_installation_key ON terminals(tenant_id,installation_id)`,
		`ALTER TABLE terminals ADD CONSTRAINT terminals_tenant_id_key UNIQUE(tenant_id,id)`,
		`ALTER TABLE terminals ADD CONSTRAINT terminals_enrolled_membership_fk FOREIGN KEY(tenant_id,enrolled_by) REFERENCES tenant_memberships(tenant_id,user_id)`,
		`ALTER TABLE terminals ALTER COLUMN tenant_id DROP DEFAULT`,
		`ALTER TABLE sessions ADD COLUMN context_kind text NOT NULL DEFAULT 'tenant' CHECK(context_kind IN ('account','tenant','platform'))`,
		fmt.Sprintf(`ALTER TABLE sessions ADD COLUMN tenant_id uuid DEFAULT '%s' REFERENCES tenants(id)`, initialTenantID),
		`ALTER TABLE sessions ADD COLUMN membership_id uuid`,
		`ALTER TABLE sessions ADD COLUMN legacy_origin boolean NOT NULL DEFAULT true`,
		`ALTER TABLE sessions ALTER COLUMN legacy_origin SET DEFAULT false`,
		`UPDATE sessions s SET membership_id=m.id FROM tenant_memberships m WHERE m.user_id=s.user_id AND m.tenant_id=s.tenant_id`,
		`ALTER TABLE sessions ALTER COLUMN data_space_id DROP NOT NULL`,
		`ALTER TABLE sessions ALTER COLUMN data_space_id DROP DEFAULT`,
		`ALTER TABLE sessions ALTER COLUMN tenant_id DROP DEFAULT`,
		`ALTER TABLE sessions ADD CONSTRAINT sessions_tenant_id_key UNIQUE(tenant_id,id)`,
		`ALTER TABLE sessions ADD CONSTRAINT sessions_context_shape CHECK (
			(context_kind='tenant' AND tenant_id IS NOT NULL AND membership_id IS NOT NULL AND data_space_id IS NOT NULL) OR
			(context_kind IN ('account','platform') AND tenant_id IS NULL AND membership_id IS NULL AND data_space_id IS NULL AND terminal_id IS NULL)
		)`,
		`ALTER TABLE sessions ADD CONSTRAINT sessions_membership_tenant_fk FOREIGN KEY(membership_id,tenant_id,user_id) REFERENCES tenant_memberships(id,tenant_id,user_id)`,
		`ALTER TABLE sessions ADD CONSTRAINT sessions_space_tenant_fk FOREIGN KEY(tenant_id,data_space_id) REFERENCES data_spaces(tenant_id,id)`,
		`ALTER TABLE sessions ADD CONSTRAINT sessions_terminal_tenant_fk FOREIGN KEY(tenant_id,terminal_id) REFERENCES terminals(tenant_id,id)`,
		`CREATE TABLE tenant_invitations (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(), tenant_id uuid NOT NULL REFERENCES tenants(id),
			role text NOT NULL CHECK(role IN ('admin','superadmin')),
			code_hash bytea NOT NULL UNIQUE, expires_at timestamptz NOT NULL,
			created_by uuid NOT NULL REFERENCES users(id), accepted_by uuid REFERENCES users(id),
			accepted_at timestamptz, revoked_at timestamptz,
			is_initial_owner boolean NOT NULL DEFAULT false,
			created_at timestamptz NOT NULL DEFAULT now(),
			CHECK((accepted_at IS NULL) = (accepted_by IS NULL)),
			CHECK(NOT is_initial_owner OR role='superadmin')
		)`,
		`CREATE INDEX tenant_invitations_pending_idx ON tenant_invitations(tenant_id,expires_at) WHERE accepted_at IS NULL AND revoked_at IS NULL`,
		`CREATE TABLE tenant_profile_revisions (
			tenant_id uuid NOT NULL REFERENCES tenants(id), revision integer NOT NULL CHECK(revision>0),
			business_name text NOT NULL CHECK(length(btrim(business_name)) BETWEEN 1 AND 160),
			address text NOT NULL DEFAULT '' CHECK(length(address)<=500), phone text NOT NULL DEFAULT '' CHECK(length(phone)<=50),
			created_by uuid REFERENCES users(id), created_at timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY(tenant_id,revision)
		)`,
		fmt.Sprintf(`INSERT INTO tenant_profile_revisions(tenant_id,revision,business_name) VALUES ('%s',1,'Telomoyo')`, initialTenantID),
		`CREATE TABLE tenant_qris_revisions (
			tenant_id uuid NOT NULL REFERENCES tenants(id), revision integer NOT NULL CHECK(revision>0),
			payload_hash text NOT NULL CHECK(payload_hash ~ '^[0-9a-f]{64}$'),
			static_payload text NOT NULL CHECK(length(static_payload) BETWEEN 1 AND 4096),
			created_by uuid NOT NULL REFERENCES users(id), created_at timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY(tenant_id,revision)
		)`,
		`ALTER TABLE tenants ADD CONSTRAINT tenants_profile_revision_fk FOREIGN KEY(id,profile_revision) REFERENCES tenant_profile_revisions(tenant_id,revision) DEFERRABLE INITIALLY DEFERRED`,
		`ALTER TABLE tenants ADD CONSTRAINT tenants_qris_revision_fk FOREIGN KEY(id,qris_revision) REFERENCES tenant_qris_revisions(tenant_id,revision) DEFERRABLE INITIALLY DEFERRED`,
		`CREATE TABLE platform_audit_events (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(), event_type text NOT NULL,
			actor_id uuid REFERENCES users(id), tenant_id uuid REFERENCES tenants(id),
			metadata jsonb NOT NULL DEFAULT '{}', created_at timestamptz NOT NULL DEFAULT now()
		)`,
		// Freeze the original business identity without updating append-only rows.
		`ALTER TABLE transactions ADD COLUMN receipt_profile_revision integer NOT NULL DEFAULT 1`,
		`ALTER TABLE transactions ADD COLUMN receipt_identity jsonb NOT NULL DEFAULT '{"businessName":"Telomoyo","address":"","phone":"","revision":1}'`,
		`ALTER TABLE transaction_revisions ADD COLUMN receipt_identity jsonb NOT NULL DEFAULT '{"businessName":"Telomoyo","address":"","phone":"","revision":1}'`,
		`ALTER TABLE transactions ALTER COLUMN receipt_profile_revision DROP DEFAULT`,
		`ALTER TABLE transactions ALTER COLUMN receipt_identity DROP DEFAULT`,
		`ALTER TABLE transaction_revisions ALTER COLUMN receipt_identity DROP DEFAULT`,
	}
	for _, statement := range statements {
		if err := tx.WithContext(ctx).Exec(statement).Error; err != nil {
			return fmt.Errorf("apply tenant schema: %w", err)
		}
	}
	for _, table := range []string{"packages", "package_revisions", "transactions", "transaction_revisions", "transaction_items", "print_attempts", "audit_events", "sync_changes", "idempotency_records"} {
		for _, statement := range []string{
			fmt.Sprintf(`ALTER TABLE %s ADD COLUMN tenant_id uuid NOT NULL DEFAULT '%s'`, table, initialTenantID),
			fmt.Sprintf(`ALTER TABLE %s ADD CONSTRAINT %s_tenant_space_fk FOREIGN KEY(tenant_id,data_space_id) REFERENCES data_spaces(tenant_id,id)`, table, table),
			fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN tenant_id DROP DEFAULT`, table),
			fmt.Sprintf(`ALTER TABLE %s ALTER COLUMN data_space_id DROP DEFAULT`, table),
		} {
			if err := tx.WithContext(ctx).Exec(statement).Error; err != nil {
				return fmt.Errorf("scope %s: %w", table, err)
			}
		}
	}
	for _, spec := range []struct{ table, column, target string }{
		{"packages", "created_by", "tenant_memberships(tenant_id,user_id)"}, {"packages", "updated_by", "tenant_memberships(tenant_id,user_id)"}, {"packages", "deleted_by", "tenant_memberships(tenant_id,user_id)"},
		{"package_revisions", "created_by", "tenant_memberships(tenant_id,user_id)"},
		{"transactions", "origin_actor_id", "tenant_memberships(tenant_id,user_id)"}, {"transactions", "updated_by", "tenant_memberships(tenant_id,user_id)"}, {"transactions", "deleted_by", "tenant_memberships(tenant_id,user_id)"},
		{"transaction_revisions", "origin_actor_id", "tenant_memberships(tenant_id,user_id)"}, {"transaction_revisions", "submitted_by_actor_id", "tenant_memberships(tenant_id,user_id)"},
		{"print_attempts", "actor_id", "tenant_memberships(tenant_id,user_id)"},
		{"transactions", "terminal_id", "terminals(tenant_id,id)"}, {"transaction_revisions", "terminal_id", "terminals(tenant_id,id)"}, {"print_attempts", "terminal_id", "terminals(tenant_id,id)"}, {"audit_events", "terminal_id", "terminals(tenant_id,id)"}, {"idempotency_records", "terminal_id", "terminals(tenant_id,id)"},
	} {
		q := fmt.Sprintf(`ALTER TABLE %s ADD CONSTRAINT %s_%s_tenant_fk FOREIGN KEY(tenant_id,%s) REFERENCES %s`, spec.table, spec.table, spec.column, spec.column, spec.target)
		if err := tx.WithContext(ctx).Exec(q).Error; err != nil {
			return fmt.Errorf("tenant reference %s: %w", spec.table, err)
		}
	}
	statements = []string{
		`ALTER TABLE packages ADD CONSTRAINT packages_tenant_id_key UNIQUE(tenant_id,id)`,
		`ALTER TABLE packages ADD CONSTRAINT packages_source_tenant_fk FOREIGN KEY(tenant_id,source_package_id) REFERENCES packages(tenant_id,id)`,
		`ALTER TABLE transactions ADD CONSTRAINT transactions_receipt_profile_fk FOREIGN KEY(tenant_id,receipt_profile_revision) REFERENCES tenant_profile_revisions(tenant_id,revision)`,
		`CREATE OR REPLACE FUNCTION bind_data_space_tenant() RETURNS trigger LANGUAGE plpgsql AS $$
		DECLARE owner_tenant uuid;
		BEGIN
			SELECT tenant_id INTO STRICT owner_tenant FROM data_spaces WHERE id=NEW.data_space_id;
			IF NEW.tenant_id IS NOT NULL AND NEW.tenant_id<>owner_tenant THEN RAISE EXCEPTION 'tenant/data space mismatch' USING ERRCODE='23514'; END IF;
			NEW.tenant_id:=owner_tenant; RETURN NEW;
		END $$`,
		`CREATE OR REPLACE FUNCTION bind_session_tenant() RETURNS trigger LANGUAGE plpgsql AS $$
		DECLARE owner_tenant uuid;
		BEGIN
			IF NEW.context_kind='tenant' THEN
				SELECT tenant_id INTO STRICT owner_tenant FROM data_spaces WHERE id=NEW.data_space_id;
				IF NEW.tenant_id IS NOT NULL AND NEW.tenant_id<>owner_tenant THEN RAISE EXCEPTION 'session tenant mismatch' USING ERRCODE='23514'; END IF;
				NEW.tenant_id:=owner_tenant;
				IF NEW.membership_id IS NULL THEN SELECT id INTO STRICT NEW.membership_id FROM tenant_memberships WHERE tenant_id=owner_tenant AND user_id=NEW.user_id; END IF;
			END IF;
			RETURN NEW;
		END $$`,
		`CREATE TRIGGER sessions_bind_tenant BEFORE INSERT ON sessions FOR EACH ROW EXECUTE FUNCTION bind_session_tenant()`,
		`CREATE OR REPLACE FUNCTION protect_session_scope() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
			IF (NEW.user_id,NEW.context_kind,NEW.tenant_id,NEW.membership_id,NEW.data_space_id,NEW.legacy_origin) IS DISTINCT FROM (OLD.user_id,OLD.context_kind,OLD.tenant_id,OLD.membership_id,OLD.data_space_id,OLD.legacy_origin) THEN
				RAISE EXCEPTION 'session scope is immutable' USING ERRCODE='55000';
			END IF;
			IF OLD.terminal_id IS NOT NULL AND NEW.terminal_id IS DISTINCT FROM OLD.terminal_id THEN
				RAISE EXCEPTION 'session enrollment is immutable' USING ERRCODE='55000';
			END IF; RETURN NEW;
		END $$`,
		`CREATE TRIGGER sessions_immutable_scope BEFORE UPDATE ON sessions FOR EACH ROW EXECUTE FUNCTION protect_session_scope()`,
		`CREATE OR REPLACE FUNCTION protect_data_space_identity() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
			IF (NEW.id,NEW.tenant_id,NEW.mode,NEW.generation) IS DISTINCT FROM (OLD.id,OLD.tenant_id,OLD.mode,OLD.generation) THEN
				RAISE EXCEPTION 'data space identity is immutable' USING ERRCODE='55000';
			END IF; RETURN NEW; END $$`,
		`CREATE TRIGGER data_spaces_immutable_identity BEFORE UPDATE ON data_spaces FOR EACH ROW EXECUTE FUNCTION protect_data_space_identity()`,
		`ALTER TABLE sync_changes DROP CONSTRAINT sync_changes_aggregate_allowed`,
		`ALTER TABLE sync_changes ADD CONSTRAINT sync_changes_aggregate_allowed CHECK(aggregate IN ('user','package','transaction','print_attempt','terminal','tenant_profile','tenant_qris'))`,
	}
	for _, statement := range statements {
		if err := tx.WithContext(ctx).Exec(statement).Error; err != nil {
			return fmt.Errorf("tenant guards: %w", err)
		}
	}
	for _, table := range []string{"packages", "package_revisions", "transactions", "transaction_revisions", "transaction_items", "print_attempts", "audit_events", "sync_changes", "idempotency_records"} {
		q := fmt.Sprintf(`CREATE TRIGGER %s_bind_tenant BEFORE INSERT ON %s FOR EACH ROW EXECUTE FUNCTION bind_data_space_tenant()`, table, table)
		if err := tx.WithContext(ctx).Exec(q).Error; err != nil {
			return err
		}
	}
	for _, table := range []string{"tenant_profile_revisions", "tenant_qris_revisions", "platform_audit_events"} {
		q := fmt.Sprintf(`CREATE TRIGGER %s_append_only BEFORE UPDATE OR DELETE ON %s FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation()`, table, table)
		if err := tx.WithContext(ctx).Exec(q).Error; err != nil {
			return err
		}
	}
	return nil
}
