package migrations

import (
	"context"
	"fmt"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/observability"
	"gorm.io/gorm"
)

// Existing membership/session identifiers remain historical evidence. No signed
// queue or append-only transaction snapshot is rewritten by this migration.
func migrateInternalOrganization(ctx context.Context, tx *gorm.DB) error {
	defer observability.StartSegment(ctx, "Migrations.InternalOrganization")()
	statements := []string{
		`ALTER TABLE sessions ADD COLUMN protocol_version integer NOT NULL DEFAULT 2 CHECK(protocol_version BETWEEN 1 AND 3)`,
		`ALTER TABLE sessions ADD COLUMN sandbox_qris_policy text NOT NULL DEFAULT 'fixed_1000' CHECK(sandbox_qris_policy IN ('fixed_1000','transaction_total'))`,
		`ALTER TABLE sessions ADD CONSTRAINT sessions_policy_protocol CHECK(sandbox_qris_policy <> 'transaction_total' OR protocol_version >= 3)`,
		`ALTER TABLE tenants ADD COLUMN management_revision integer NOT NULL DEFAULT 1 CHECK(management_revision > 0)`,
		`INSERT INTO platform_audit_events(event_type,actor_id,metadata)
		 SELECT 'account.global_role_migrated',NULL,jsonb_build_object('accountId',u.id,'before',u.role,'after',
		 CASE WHEN u.is_platform_admin OR EXISTS(SELECT 1 FROM tenant_memberships m WHERE m.user_id=u.id AND m.status='active' AND m.role='superadmin') THEN 'superadmin' ELSE 'admin' END)
		 FROM users u WHERE u.is_active AND u.deleted_at IS NULL`,
		`UPDATE users u SET role=CASE WHEN u.is_platform_admin OR EXISTS(SELECT 1 FROM tenant_memberships m WHERE m.user_id=u.id AND m.status='active' AND m.role='superadmin') THEN 'superadmin' ELSE 'admin' END,
		 updated_at=now() WHERE u.is_active AND u.deleted_at IS NULL`,
		`UPDATE tenant_invitations SET revoked_at=now() WHERE accepted_at IS NULL AND revoked_at IS NULL`,
		`INSERT INTO platform_audit_events(event_type,metadata) VALUES('organization.shared_access_enabled','{"roles":["admin","superadmin"],"invitationsEnabled":false}'::jsonb)`,
		`CREATE OR REPLACE FUNCTION protect_session_scope() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
		 IF (NEW.user_id,NEW.context_kind,NEW.tenant_id,NEW.membership_id,NEW.data_space_id,NEW.legacy_origin,NEW.protocol_version,NEW.sandbox_qris_policy)
		 IS DISTINCT FROM (OLD.user_id,OLD.context_kind,OLD.tenant_id,OLD.membership_id,OLD.data_space_id,OLD.legacy_origin,OLD.protocol_version,OLD.sandbox_qris_policy) THEN
		 RAISE EXCEPTION 'session scope and payment policy are immutable' USING ERRCODE='55000'; END IF;
		 IF OLD.terminal_id IS NOT NULL AND NEW.terminal_id IS DISTINCT FROM OLD.terminal_id THEN
		 RAISE EXCEPTION 'session enrollment is immutable' USING ERRCODE='55000'; END IF;
		 RETURN NEW; END $$`,
		`ALTER TABLE sync_changes DROP CONSTRAINT sync_changes_aggregate_allowed`,
		`ALTER TABLE sync_changes ADD CONSTRAINT sync_changes_aggregate_allowed CHECK(aggregate IN ('user','package','transaction','print_attempt','terminal','tenant_profile','tenant_qris','tenant_metadata'))`,
		// Publish fresh safe projections after the role mapping. Existing numeric
		// cursors and historical events remain untouched; even an already-synced
		// device receives the organization-wide role and management name.
		`INSERT INTO sync_changes(data_space_id,tenant_id,aggregate,aggregate_id,action,revision,payload)
		 SELECT ds.id,t.id,'tenant_metadata',t.id::text,'updated',t.management_revision,
		 jsonb_build_object('id',t.id,'name',t.name,'slug',t.slug,'status',t.status,'revision',t.management_revision,'createdAt',t.created_at,'updatedAt',t.updated_at)
		 FROM data_spaces ds JOIN tenants t ON t.id=ds.tenant_id WHERE ds.status='active'`,
		`INSERT INTO sync_changes(data_space_id,tenant_id,aggregate,aggregate_id,action,payload,tombstone)
		 SELECT ds.id,ds.tenant_id,'user',u.id::text,
		 CASE WHEN u.is_active AND u.deleted_at IS NULL THEN 'updated' ELSE 'deleted' END,
		 jsonb_build_object('id',u.id,'fullName',u.full_name,'username',u.username,'role',u.role,
		 'active',u.is_active AND u.deleted_at IS NULL,'membershipId',m.id,'tenantId',ds.tenant_id,
		 'mustChangePassword',u.must_change_password,'createdAt',u.created_at,'updatedAt',u.updated_at,'deletedAt',u.deleted_at),
		 NOT u.is_active OR u.deleted_at IS NOT NULL
		 FROM data_spaces ds CROSS JOIN users u
		 LEFT JOIN tenant_memberships m ON m.tenant_id=ds.tenant_id AND m.user_id=u.id
		 WHERE ds.status='active'`,
	}
	for _, statement := range statements {
		if err := tx.WithContext(ctx).Exec(statement).Error; err != nil {
			return fmt.Errorf("internal organization migration: %w", err)
		}
	}
	return nil
}
