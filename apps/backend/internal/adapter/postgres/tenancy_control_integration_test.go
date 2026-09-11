package postgres

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/google/uuid"
)

func TestTenantProvisioningInvitationMembershipAndRecovery(t *testing.T) {
	store := openSandboxLifecycleStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	legacy, _ := seedSandboxLifecyclePrincipal(t, ctx, store)
	if _, err := store.ListTenants(ctx, legacy); !domain.IsCode(err, domain.CodeForbidden) {
		t.Fatalf("tenant owner inherited platform permission: %v", err)
	}
	if _, err := store.Pool.Exec(ctx, `UPDATE users SET is_platform_admin=true WHERE id=$1`, legacy.UserID); err != nil {
		t.Fatal(err)
	}
	platform, err := store.SwitchContextSession(ctx, legacy, integrationBytes(23), integrationBytes(130), domain.SwitchContextInput{Kind: domain.ContextPlatform})
	if err != nil {
		t.Fatal(err)
	}
	if platform.IsTenantContext() || platform.DataSpaceID != uuid.Nil || platform.TerminalID != nil {
		t.Fatal("platform context contains business authority")
	}
	created, err := store.CreateTenant(ctx, platform, domain.CreateTenantInput{Name: "Invited Business", Slug: "invited-business"}, integrationBytes(131))
	if err != nil {
		t.Fatal(err)
	}
	if created.Tenant.Status != "pending_setup" || !created.Invitation.InitialOwner {
		t.Fatalf("unexpected provision: %+v", created)
	}
	if _, err = store.SwitchContextSession(ctx, platform, integrationBytes(130), integrationBytes(132), domain.SwitchContextInput{Kind: domain.ContextTenant, TenantID: created.Tenant.ID}); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("platform gained business access without membership: %v", err)
	}
	var userID uuid.UUID
	if err = store.Pool.QueryRow(ctx, `INSERT INTO users(full_name,username,password_hash,role,must_change_password) VALUES('Business Owner','business.owner','hash','admin',false) RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	account, err := store.CreateAccountSession(ctx, userID, integrationBytes(133))
	if err != nil {
		t.Fatal(err)
	}
	if account.ContextKind != domain.ContextAccount || account.DataSpaceID != uuid.Nil {
		t.Fatal("unaffiliated account received tenant context")
	}
	accepted, err := store.AcceptTenantInvitation(ctx, account, integrationBytes(131))
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Tenant.Status != "active" || accepted.Role != domain.RoleSuperadmin {
		t.Fatalf("owner invitation did not activate tenant: %+v", accepted)
	}
	if _, err = store.AcceptTenantInvitation(ctx, account, integrationBytes(131)); !domain.IsCode(err, domain.CodeInvitationInvalid) {
		t.Fatalf("invitation reused: %v", err)
	}
	owner, err := store.SwitchContextSession(ctx, account, integrationBytes(133), integrationBytes(134), domain.SwitchContextInput{Kind: domain.ContextTenant, TenantID: created.Tenant.ID})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := store.UpdateTenantProfile(ctx, owner, domain.UpdateTenantProfileInput{ExpectedRevision: 1, BusinessName: "Business Revision 2", Address: "Second address", Phone: "081234"})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Revision != 2 {
		t.Fatalf("profile revision=%d", profile.Revision)
	}
	if _, err = store.UpdateTenantProfile(ctx, owner, domain.UpdateTenantProfileInput{ExpectedRevision: 1, BusinessName: "Stale"}); !domain.IsCode(err, domain.CodeRevisionConflict) {
		t.Fatalf("stale profile update: %v", err)
	}
	for revision := 0; revision < 2; revision++ {
		qris, e := store.UpdateTenantQRIS(ctx, owner, domain.UpdateTenantQRISInput{ExpectedRevision: revision, StaticPayload: "immutable-fixture"}, strings.Repeat("b", 64))
		if e != nil {
			t.Fatal(e)
		}
		if qris.Revision != revision+1 || len(qris.Payloads) != revision+1 {
			t.Fatalf("QRIS did not version reactivation: %+v", qris)
		}
	}
	if _, err = store.CreateTenantInvitation(ctx, platform, created.Tenant.ID, domain.RoleSuperadmin, true, integrationBytes(135)); !domain.IsCode(err, domain.CodeConflict) {
		t.Fatalf("platform reissued active tenant owner access: %v", err)
	}
	if _, err = store.CreateTenantInvitation(ctx, owner, owner.TenantID, domain.RoleAdmin, false, integrationBytes(136)); err != nil {
		t.Fatal(err)
	}
	if _, err = store.AcceptTenantInvitation(ctx, platform, integrationBytes(136)); err != nil {
		t.Fatal(err)
	}
	staff, err := store.SwitchContextSession(ctx, platform, integrationBytes(130), integrationBytes(137), domain.SwitchContextInput{Kind: domain.ContextTenant, TenantID: owner.TenantID})
	if err != nil {
		t.Fatal(err)
	}
	if staff.Role != domain.RoleAdmin {
		t.Fatal("global superadmin role overrode tenant admin membership")
	}
	if _, err = store.UpdateTenantProfile(ctx, staff, domain.UpdateTenantProfileInput{ExpectedRevision: 2, BusinessName: "Not permitted"}); !domain.IsCode(err, domain.CodeForbidden) {
		t.Fatalf("tenant admin updated profile: %v", err)
	}
	inactive := false
	if _, err = store.UpdateTenantMember(ctx, owner, staff.UserID, domain.UpdateMembershipInput{Active: &inactive}); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.PrincipalBySession(ctx, staff.SessionID, integrationBytes(137))
	if !domain.IsCode(err, domain.CodeMembershipRevoked) {
		t.Fatalf("revoked membership returned %v", err)
	}
	initialMember, err := store.GetUser(ctx, domain.InitialTenantID(), staff.UserID)
	if err != nil || initialMember.Role != domain.RoleSuperadmin || !initialMember.IsActive {
		t.Fatalf("other membership was modified: %+v %v", initialMember, err)
	}
	back, err := store.SwitchContextSession(ctx, recovered, integrationBytes(137), integrationBytes(138), domain.SwitchContextInput{Kind: domain.ContextPlatform})
	if err != nil {
		t.Fatalf("revoked tenant blocked context recovery: %v", err)
	}
	if _, err = store.SetTenantStatus(ctx, back, owner.TenantID, "suspended"); err != nil {
		t.Fatal(err)
	}
	blocked, err := store.PrincipalBySession(ctx, owner.SessionID, integrationBytes(134))
	if !domain.IsCode(err, domain.CodeTenantSuspended) {
		t.Fatalf("suspension not discovered: %v", err)
	}
	if _, err = store.SetTenantStatus(ctx, back, owner.TenantID, "active"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PrincipalBySession(ctx, owner.SessionID, integrationBytes(134)); !domain.IsCode(err, domain.CodeMembershipInactive) {
		t.Fatalf("reactivation revived revoked session: %v", err)
	}
	if _, err = store.SwitchContextSession(ctx, blocked, integrationBytes(134), integrationBytes(139), domain.SwitchContextInput{Kind: domain.ContextTenant, TenantID: owner.TenantID}); err != nil {
		t.Fatalf("reactivated tenant could not exchange fresh session: %v", err)
	}
	audit, err := store.ListPlatformAudit(ctx, back, 100)
	if err != nil || len(audit) < 8 {
		t.Fatalf("missing durable control audit: %d %v", len(audit), err)
	}
}

func TestInvitationAtomicConsumptionAndQuarantineOriginValidation(t *testing.T) {
	store := openSandboxLifecycleStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	actor, _ := seedSandboxLifecyclePrincipal(t, ctx, store)
	other, _ := seedSandboxLifecyclePrincipalWithToken(t, ctx, store, integrationBytes(151))
	invitation, err := store.CreateTenantInvitation(ctx, actor, actor.TenantID, domain.RoleAdmin, false, integrationBytes(152))
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := store.AcceptTenantInvitation(ctx, other, integrationBytes(152))
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	success, invalid := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if domain.IsCode(e, domain.CodeInvitationInvalid) {
			invalid++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || invalid != 1 {
		t.Fatalf("concurrent consumption success=%d invalid=%d", success, invalid)
	}
	if err = store.RevokeTenantInvitation(ctx, actor, invitation.ID); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("consumed invitation revocation: %v", err)
	}
	origins := []domain.OutboxOrigin{{OriginActorID: actor.UserID, OriginSessionID: actor.SessionID, TerminalID: *actor.TerminalID}, {OriginActorID: other.UserID, OriginSessionID: other.SessionID, TerminalID: *other.TerminalID}, {OriginActorID: actor.UserID, OriginSessionID: uuid.New(), TerminalID: *actor.TerminalID}}
	validated, err := store.RevalidateOrigins(ctx, actor, origins)
	if err != nil {
		t.Fatal(err)
	}
	if len(validated) != 3 || !validated[0].Allowed || validated[1].Allowed || validated[2].Allowed {
		t.Fatalf("quarantine release crossed origin ownership: %+v", validated)
	}
	if err = store.ChangeOwnPassword(ctx, actor, "new-hash"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateUser(ctx, actor, domain.CreateUserInput{}, "hash"); !domain.IsCode(err, domain.CodeClientUpdateRequired) {
		t.Fatalf("obsolete global mutation reachable: %v", err)
	}
}
