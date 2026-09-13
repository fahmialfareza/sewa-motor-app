package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/adapter/security"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/bootstrap"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/google/uuid"
)

func TestBootstrapOrganizationProjectionAndRecoveryReachAllActiveSpaces(t *testing.T) {
	store := openSandboxLifecycleStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	owner, _ := seedSandboxLifecyclePrincipal(t, ctx, store)
	manager, err := store.CreateAccountSessionWithProtocol(ctx, owner.UserID, integrationBytes(210), 3)
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateTenant(ctx, manager, domain.CreateTenantInput{Name: "Shared business", Slug: "shared-business"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.EnsureSandbox(ctx, other.Tenant.ID); err != nil {
		t.Fatal(err)
	}
	manifest, err := bootstrap.NewSampleSuperadminManifest("Operator sample", "operator.sample", "temporary-fixture-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = bootstrap.Apply(ctx, store.Pool, security.DefaultArgon2id(), manifest); err != nil {
		t.Fatal(err)
	}
	var accountID uuid.UUID
	if err = store.Pool.QueryRow(ctx, `SELECT id FROM users WHERE username='operator.sample'`).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	old, err := store.CreateAccountSessionWithProtocol(ctx, accountID, integrationBytes(211), 3)
	if err != nil {
		t.Fatal(err)
	}
	if err = bootstrap.ResetSampleSuperadminPassword(ctx, store.Pool, security.DefaultArgon2id(), manifest); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PrincipalBySession(ctx, old.SessionID, integrationBytes(211)); !domain.IsCode(err, domain.CodeAccountAccessChanged) {
		t.Fatalf("recovery did not preserve quarantine identity: %v", err)
	}
	var activeSpaces, projectedSpaces, events int
	var safe bool
	if err = store.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM data_spaces WHERE status='active'),count(DISTINCT data_space_id),
	 bool_and(payload->>'role'='superadmin' AND payload->>'tenantId'=tenant_id::text AND NOT jsonb_exists(payload,'passwordHash') AND NOT jsonb_exists(payload,'password_hash'))
	 FROM sync_changes WHERE aggregate='user' AND aggregate_id=$1`, accountID.String()).Scan(&activeSpaces, &projectedSpaces, &safe); err != nil {
		t.Fatal(err)
	}
	if projectedSpaces != activeSpaces || !safe {
		t.Fatalf("safe global projections=%d want%d safe=%v", projectedSpaces, activeSpaces, safe)
	}
	if err = store.Pool.QueryRow(ctx, `SELECT count(*) FROM platform_audit_events WHERE metadata->>'accountId'=$1 AND event_type IN ('account.bootstrapped','account.password_recovered')`, accountID.String()).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 2 {
		t.Fatalf("global operator audit events=%d", events)
	}
	var links int
	if err = store.Pool.QueryRow(ctx, `SELECT count(*) FROM tenant_memberships WHERE tenant_id=$1 AND user_id=$2`, other.Tenant.ID, accountID).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if links != 0 {
		t.Fatal("projection created an unnecessary approval link")
	}
}
