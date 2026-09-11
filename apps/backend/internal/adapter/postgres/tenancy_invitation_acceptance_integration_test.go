package postgres

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/google/uuid"
)

func TestTenantInvitationExpiryRevocationAndRegistrationRollback(t *testing.T) {
	store := openSandboxLifecycleStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	owner, _ := seedSandboxLifecyclePrincipal(t, ctx, store)
	var accountID uuid.UUID
	if err := store.Pool.QueryRow(ctx, `INSERT INTO users(full_name,username,password_hash,role,must_change_password) VALUES('Existing Invitee','existing.invitee','hash','admin',false) RETURNING id`).Scan(&accountID); err != nil {
		t.Fatal(err)
	}
	account, err := store.CreateAccountSession(ctx, accountID, integrationBytes(170))
	if err != nil {
		t.Fatal(err)
	}
	for i, state := range []string{"expired", "revoked"} {
		t.Run(state, func(t *testing.T) {
			hash := integrationBytes(byte(171 + i))
			invitation, e := store.CreateTenantInvitation(ctx, owner, owner.TenantID, domain.RoleAdmin, false, hash)
			if e != nil {
				t.Fatal(e)
			}
			if invitation.ExpiresAt.Sub(invitation.CreatedAt) != 7*24*time.Hour {
				t.Fatalf("invitation retention=%s, want seven days", invitation.ExpiresAt.Sub(invitation.CreatedAt))
			}
			if state == "expired" {
				_, e = store.Pool.Exec(ctx, `UPDATE tenant_invitations SET expires_at=now()-interval '1 second' WHERE id=$1`, invitation.ID)
			} else {
				e = store.RevokeTenantInvitation(ctx, owner, invitation.ID)
			}
			if e != nil {
				t.Fatal(e)
			}
			if _, e = store.AcceptTenantInvitation(ctx, account, hash); !domain.IsCode(e, domain.CodeInvitationInvalid) {
				t.Fatalf("%s invitation accepted by account: %v", state, e)
			}
			username := "new." + state
			if _, e = store.RegisterTenantInvitation(ctx, domain.RegisterInvitationInput{FullName: "New Invitee", Username: username}, "new-hash", hash); !domain.IsCode(e, domain.CodeInvitationInvalid) {
				t.Fatalf("%s invitation registered account: %v", state, e)
			}
			var users, memberships, acceptances int
			if e = store.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM users WHERE username=$1),(SELECT count(*) FROM tenant_memberships WHERE user_id=$2),(SELECT count(*) FROM tenant_invitations WHERE id=$3 AND (accepted_at IS NOT NULL OR accepted_by IS NOT NULL))`, username, accountID, invitation.ID).Scan(&users, &memberships, &acceptances); e != nil {
				t.Fatal(e)
			}
			if users != 0 || memberships != 0 || acceptances != 0 {
				t.Fatalf("failed invitation left users=%d memberships=%d acceptances=%d", users, memberships, acceptances)
			}
		})
	}
	// A username collision must not consume an otherwise valid invitation.
	invitation, err := store.CreateTenantInvitation(ctx, owner, owner.TenantID, domain.RoleAdmin, false, integrationBytes(173))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.RegisterTenantInvitation(ctx, domain.RegisterInvitationInput{FullName: "Duplicate", Username: "existing.invitee"}, "new-hash", integrationBytes(173)); !domain.IsCode(err, domain.CodeConflict) {
		t.Fatalf("duplicate username registration: %v", err)
	}
	createdID, err := store.RegisterTenantInvitation(ctx, domain.RegisterInvitationInput{FullName: "Registered Staff", Username: "registered.staff"}, "registered-hash", integrationBytes(173))
	if err != nil {
		t.Fatalf("failed registration consumed the invitation: %v", err)
	}
	var acceptedID uuid.UUID
	var role domain.Role
	var password string
	var mustChange bool
	if err = store.Pool.QueryRow(ctx, `SELECT i.accepted_by,m.role,u.password_hash,u.must_change_password FROM tenant_invitations i JOIN tenant_memberships m ON m.tenant_id=i.tenant_id AND m.user_id=i.accepted_by JOIN users u ON u.id=m.user_id WHERE i.id=$1 AND i.accepted_at IS NOT NULL`, invitation.ID).Scan(&acceptedID, &role, &password, &mustChange); err != nil {
		t.Fatal(err)
	}
	if acceptedID != createdID || role != domain.RoleAdmin || password != "registered-hash" || mustChange {
		t.Fatal("invited registration did not atomically establish the expected account and membership")
	}
}

func TestInitialOwnerReissueAndRegistrationActivationAtomicity(t *testing.T) {
	store := openSandboxLifecycleStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	owner, _ := seedSandboxLifecyclePrincipal(t, ctx, store)
	if _, err := store.Pool.Exec(ctx, `UPDATE users SET is_platform_admin=true WHERE id=$1`, owner.UserID); err != nil {
		t.Fatal(err)
	}
	platform, err := store.SwitchContextSession(ctx, owner, integrationBytes(23), integrationBytes(180), domain.SwitchContextInput{Kind: domain.ContextPlatform})
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateTenant(ctx, platform, domain.CreateTenantInput{Name: "Atomic Registration", Slug: "atomic-registration"}, integrationBytes(181))
	if err != nil {
		t.Fatal(err)
	}
	reissued, err := store.CreateTenantInvitation(ctx, platform, created.Tenant.ID, domain.RoleSuperadmin, true, integrationBytes(182))
	if err != nil {
		t.Fatal(err)
	}
	input := domain.RegisterInvitationInput{FullName: "Initial Owner", Username: "initial.owner"}
	if _, err = store.RegisterTenantInvitation(ctx, input, "owner-hash", integrationBytes(181)); !domain.IsCode(err, domain.CodeInvitationInvalid) {
		t.Fatalf("reissued owner code remained valid: %v", err)
	}
	// Force failure after membership creation, invitation consumption, tenant
	// activation, and initial sync seeding, using only this disposable schema.
	if _, err = store.Pool.Exec(ctx, `CREATE FUNCTION reject_test_invitation_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.event_type='invitation.accepted' THEN RAISE EXCEPTION 'injected invitation audit failure'; END IF; RETURN NEW; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool.Exec(ctx, `CREATE TRIGGER reject_test_invitation_audit BEFORE INSERT ON platform_audit_events FOR EACH ROW EXECUTE FUNCTION reject_test_invitation_audit()`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.RegisterTenantInvitation(ctx, input, "owner-hash", integrationBytes(182)); err == nil {
		t.Fatal("injected post-activation failure unexpectedly committed")
	}
	var status string
	var users, memberships, consumed, changes int
	if err = store.Pool.QueryRow(ctx, `SELECT status,(SELECT count(*) FROM users WHERE username=$2),(SELECT count(*) FROM tenant_memberships WHERE tenant_id=$1),(SELECT count(*) FROM tenant_invitations WHERE id=$3 AND (accepted_at IS NOT NULL OR accepted_by IS NOT NULL)),(SELECT count(*) FROM sync_changes WHERE tenant_id=$1) FROM tenants WHERE id=$1`, created.Tenant.ID, input.Username, reissued.ID).Scan(&status, &users, &memberships, &consumed, &changes); err != nil {
		t.Fatal(err)
	}
	if status != "pending_setup" || users != 0 || memberships != 0 || consumed != 0 || changes != 0 {
		t.Fatalf("rollback leaked status=%s users=%d memberships=%d consumed=%d sync=%d", status, users, memberships, consumed, changes)
	}
	if _, err = store.Pool.Exec(ctx, `DROP TRIGGER reject_test_invitation_audit ON platform_audit_events`); err != nil {
		t.Fatal(err)
	}
	userID, err := store.RegisterTenantInvitation(ctx, input, "owner-hash", integrationBytes(182))
	if err != nil {
		t.Fatalf("rolled-back invitation could not be retried: %v", err)
	}
	var role domain.Role
	if err = store.Pool.QueryRow(ctx, `SELECT t.status,m.role FROM tenants t JOIN tenant_memberships m ON m.tenant_id=t.id WHERE t.id=$1 AND m.user_id=$2`, created.Tenant.ID, userID).Scan(&status, &role); err != nil {
		t.Fatal(err)
	}
	if status != "active" || role != domain.RoleSuperadmin {
		t.Fatalf("initial owner registration status=%s role=%s", status, role)
	}
	if _, err = store.RegisterTenantInvitation(ctx, domain.RegisterInvitationInput{FullName: "Other", Username: "other.owner"}, "hash", integrationBytes(182)); !domain.IsCode(err, domain.CodeInvitationInvalid) {
		t.Fatalf("owner registration invitation reused: %v", err)
	}
}

func TestTenantSuperadminProtectionAndConcurrentDemotion(t *testing.T) {
	store := openSandboxLifecycleStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	a, _ := seedSandboxLifecyclePrincipal(t, ctx, store)
	b, _ := seedSandboxLifecyclePrincipalWithToken(t, ctx, store, integrationBytes(190))
	otherTenant := seedSecondTenantPrincipal(t, ctx, store, a.UserID, domain.RoleSuperadmin)
	admin, inactive := domain.RoleAdmin, false
	for _, input := range []domain.UpdateMembershipInput{{Role: &admin}, {Active: &inactive}} {
		if _, err := store.UpdateTenantMember(ctx, otherTenant, otherTenant.UserID, input); !domain.IsCode(err, domain.CodeSelfMutation) {
			t.Fatalf("superadmins in another tenant allowed removing the only owner: %v", err)
		}
	}
	if _, err := store.UpdateTenantMember(ctx, otherTenant, b.UserID, domain.UpdateMembershipInput{Role: &admin}); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("member mutation crossed tenants: %v", err)
	}
	invites := make(map[uuid.UUID]uuid.UUID)
	for i, actor := range []domain.Principal{a, b} {
		invitation, err := store.CreateTenantInvitation(ctx, actor, actor.TenantID, domain.RoleAdmin, false, integrationBytes(byte(191+i)))
		if err != nil {
			t.Fatal(err)
		}
		invites[actor.UserID] = invitation.ID
	}
	// Both principals are valid owners when the race starts. The tenant lock
	// must serialize them and recheck authority, leaving one active owner.
	results := make(chan error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, pair := range [][2]domain.Principal{{a, b}, {b, a}} {
		wg.Add(1)
		go func(actor, target domain.Principal) {
			defer wg.Done()
			<-start
			_, err := store.UpdateTenantMember(ctx, actor, target.UserID, domain.UpdateMembershipInput{Role: &admin})
			results <- err
		}(pair[0], pair[1])
	}
	close(start)
	wg.Wait()
	close(results)
	succeeded, rejected := 0, 0
	for err := range results {
		if err == nil {
			succeeded++
		} else if domain.IsCode(err, domain.CodeForbidden) || domain.IsCode(err, domain.CodeUnauthorized) {
			rejected++
		} else {
			t.Fatalf("unexpected concurrent demotion result: %v", err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("concurrent demotions succeeded=%d rejected=%d", succeeded, rejected)
	}
	var owners int
	if err := store.Pool.QueryRow(ctx, `SELECT count(*) FROM tenant_memberships WHERE tenant_id=$1 AND status='active' AND role='superadmin'`, a.TenantID).Scan(&owners); err != nil || owners != 1 {
		t.Fatalf("remaining owners=%d err=%v", owners, err)
	}
	for userID, invitationID := range invites {
		var role domain.Role
		var revoked bool
		if err := store.Pool.QueryRow(ctx, `SELECT m.role,i.revoked_at IS NOT NULL FROM tenant_memberships m JOIN tenant_invitations i ON i.tenant_id=m.tenant_id AND i.created_by=m.user_id WHERE m.tenant_id=$1 AND m.user_id=$2 AND i.id=$3`, a.TenantID, userID, invitationID).Scan(&role, &revoked); err != nil {
			t.Fatal(err)
		}
		if revoked != (role == domain.RoleAdmin) {
			t.Fatalf("demoted owner's outstanding invitation authority retained: role=%s revoked=%v", role, revoked)
		}
	}
	if _, err := store.PrincipalBySession(ctx, otherTenant.SessionID, integrationBytes(72)); err != nil {
		t.Fatalf("demotion revoked the same account's session in another tenant: %v", err)
	}
}
