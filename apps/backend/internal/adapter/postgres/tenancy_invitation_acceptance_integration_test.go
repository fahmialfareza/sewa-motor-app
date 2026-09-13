package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/google/uuid"
)

func TestRetiredInvitationWorkflowsPreserveHistoricalEvidence(t *testing.T) {
	store := openSandboxLifecycleStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	owner, _ := seedSandboxLifecyclePrincipal(t, ctx, store)
	account, err := store.CreateAccountSessionWithProtocol(ctx, owner.UserID, integrationBytes(170), 3)
	if err != nil {
		t.Fatal(err)
	}
	for i, state := range []string{"valid", "expired", "revoked", "consumed"} {
		t.Run(state, func(t *testing.T) {
			id, hash := uuid.New(), integrationBytes(byte(171+i))
			if _, e := store.Pool.Exec(ctx, `INSERT INTO tenant_invitations(id,tenant_id,code_hash,role,is_initial_owner,created_by,expires_at,revoked_at,accepted_at,accepted_by)
			 VALUES($1,$2,$3,'admin',false,$4,CASE WHEN $5='expired' THEN now()-interval '1 second' ELSE now()+interval '7 days' END,
			 CASE WHEN $5='revoked' THEN now() ELSE NULL END,CASE WHEN $5='consumed' THEN now() ELSE NULL END,CASE WHEN $5='consumed' THEN $4::uuid ELSE NULL END)`, id, owner.TenantID, hash, owner.UserID, state); e != nil {
				t.Fatal(e)
			}
			var before string
			if e := store.Pool.QueryRow(ctx, `SELECT row_to_json(i)::text FROM tenant_invitations i WHERE id=$1`, id).Scan(&before); e != nil {
				t.Fatal(e)
			}
			for _, operation := range []func() error{
				func() error {
					_, e := store.CreateTenantInvitation(ctx, owner, owner.TenantID, domain.RoleSuperadmin, true, hash)
					return e
				},
				func() error { _, e := store.ListTenantInvitations(ctx, owner); return e },
				func() error { return store.RevokeTenantInvitation(ctx, owner, id) },
				func() error { _, e := store.AcceptTenantInvitation(ctx, account, hash); return e },
				func() error {
					_, e := store.RegisterTenantInvitation(ctx, domain.RegisterInvitationInput{FullName: "Retired Signup", Username: "retired." + state}, "hash", hash)
					return e
				},
			} {
				if e := operation(); !domain.IsCode(e, "INVITATIONS_REMOVED") {
					t.Fatalf("retired %s workflow: %v", state, e)
				}
			}
			// Concurrent acceptance must never consume an old valid code either.
			results := make(chan error, 2)
			for range 2 {
				go func() { _, e := store.AcceptTenantInvitation(ctx, account, hash); results <- e }()
			}
			for range 2 {
				if e := <-results; !domain.IsCode(e, "INVITATIONS_REMOVED") {
					t.Fatalf("concurrent retired acceptance: %v", e)
				}
			}
			var after string
			var createdUsers int
			if e := store.Pool.QueryRow(ctx, `SELECT row_to_json(i)::text,(SELECT count(*) FROM users WHERE username=$2) FROM tenant_invitations i WHERE id=$1`, id, "retired."+state).Scan(&after, &createdUsers); e != nil {
				t.Fatal(e)
			}
			if before != after || createdUsers != 0 {
				t.Fatalf("retired workflow changed invitation history or created account: users=%d", createdUsers)
			}
		})
	}
}

func TestPendingTenantActivationIsAuditedAndAtomicWithoutOwnerInvitation(t *testing.T) {
	store := openSandboxLifecycleStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	owner, _ := seedSandboxLifecyclePrincipal(t, ctx, store)
	manager, err := store.CreateAccountSessionWithProtocol(ctx, owner.UserID, integrationBytes(180), 3)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateTenant(ctx, manager, domain.CreateTenantInput{Name: "Pending Migrated Tenant", Slug: "pending-migrated"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool.Exec(ctx, `UPDATE tenants SET status='pending_setup' WHERE id=$1`, created.Tenant.ID); err != nil {
		t.Fatal(err)
	}
	var beforeChanges int
	if err = store.Pool.QueryRow(ctx, `SELECT count(*) FROM sync_changes WHERE tenant_id=$1`, created.Tenant.ID).Scan(&beforeChanges); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool.Exec(ctx, `CREATE FUNCTION reject_test_activation_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type='tenant.status_changed' THEN RAISE EXCEPTION 'injected activation audit failure'; END IF; RETURN NEW; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool.Exec(ctx, `CREATE TRIGGER reject_test_activation_audit BEFORE INSERT ON platform_audit_events FOR EACH ROW EXECUTE FUNCTION reject_test_activation_audit()`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SetTenantStatus(ctx, manager, created.Tenant.ID, "active"); err == nil {
		t.Fatal("activation committed without durable audit")
	}
	var status string
	var revision, changes, invitations int
	if err = store.Pool.QueryRow(ctx, `SELECT status,management_revision,(SELECT count(*) FROM sync_changes WHERE tenant_id=$1),(SELECT count(*) FROM tenant_invitations WHERE tenant_id=$1) FROM tenants WHERE id=$1`, created.Tenant.ID).Scan(&status, &revision, &changes, &invitations); err != nil {
		t.Fatal(err)
	}
	if status != "pending_setup" || revision != 1 || changes != beforeChanges || invitations != 0 {
		t.Fatalf("activation rollback leaked status=%s revision=%d sync=%d invitations=%d", status, revision, changes, invitations)
	}
	if _, err = store.Pool.Exec(ctx, `DROP TRIGGER reject_test_activation_audit ON platform_audit_events`); err != nil {
		t.Fatal(err)
	}
	active, err := store.SetTenantStatus(ctx, manager, created.Tenant.ID, "active")
	if err != nil || active.Status != "active" || active.Revision != 2 {
		t.Fatalf("retry failed: %+v %v", active, err)
	}
	contexts, err := store.AvailableContexts(ctx, owner.UserID)
	if err != nil || len(contexts.Tenants) != 2 {
		t.Fatalf("activated tenant unavailable without invitation: %+v %v", contexts, err)
	}
}

func TestRetiredMembershipMutationsNeverChangeGlobalAccountsOrHistoricalLinks(t *testing.T) {
	store := openSandboxLifecycleStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	a, _ := seedSandboxLifecyclePrincipal(t, ctx, store)
	b, _ := seedSandboxLifecyclePrincipalWithToken(t, ctx, store, integrationBytes(190))
	otherTenant := seedSecondTenantPrincipal(t, ctx, store, a.UserID, domain.RoleAdmin)
	admin, inactive := domain.RoleAdmin, false
	var before string
	if err := store.Pool.QueryRow(ctx, `SELECT jsonb_build_object('user',row_to_json(u),'memberships',(SELECT jsonb_agg(m ORDER BY m.id) FROM tenant_memberships m WHERE m.user_id=u.id))::text FROM users u WHERE id=$1`, a.UserID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for _, actor := range []domain.Principal{a, b, otherTenant} {
		if _, err := store.TenantMembers(ctx, actor); !domain.IsCode(err, domain.CodeClientUpdateRequired) {
			t.Fatalf("obsolete member list reachable: %v", err)
		}
		for _, input := range []domain.UpdateMembershipInput{{Role: &admin}, {Active: &inactive}} {
			if _, err := store.UpdateTenantMember(ctx, actor, a.UserID, input); !domain.IsCode(err, domain.CodeClientUpdateRequired) {
				t.Fatalf("old membership endpoint changed global role/status: %v", err)
			}
		}
	}
	var after string
	if err := store.Pool.QueryRow(ctx, `SELECT jsonb_build_object('user',row_to_json(u),'memberships',(SELECT jsonb_agg(m ORDER BY m.id) FROM tenant_memberships m WHERE m.user_id=u.id))::text FROM users u WHERE id=$1`, a.UserID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("obsolete membership calls changed global account or historical membership evidence")
	}
	if _, err := store.PrincipalBySession(ctx, otherTenant.SessionID, integrationBytes(72)); err != nil {
		t.Fatalf("obsolete call revoked another session: %v", err)
	}
}
