package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/google/uuid"
)

func TestOrganizationManagementSharedAccessAndAccountLifecycle(t *testing.T) {
	store := openSandboxLifecycleStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	owner, _ := seedSandboxLifecyclePrincipal(t, ctx, store)
	manager, err := store.CreateAccountSessionWithProtocol(ctx, owner.UserID, integrationBytes(220), 3)
	if err != nil {
		t.Fatal(err)
	}
	if manager.Role != domain.RoleSuperadmin || manager.IsPlatformAdmin {
		t.Fatalf("global role/retired platform permission: %+v", manager)
	}
	created, err := store.CreateTenant(ctx, manager, domain.CreateTenantInput{Name: "Kios Telomoyo", Slug: "kios-telomoyo"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tenant := created.Tenant
	if tenant.Status != "active" || tenant.Revision != 1 || created.Invitation.ID != uuid.Nil {
		t.Fatalf("tenant needs invitation: %+v", created)
	}
	var packages, invitations int
	if err = store.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM packages WHERE tenant_id=$1),(SELECT count(*) FROM tenant_invitations WHERE tenant_id=$1)`, tenant.ID).Scan(&packages, &invitations); err != nil {
		t.Fatal(err)
	}
	if packages != 0 || invitations != 0 {
		t.Fatalf("new tenant has packages=%d invitations=%d", packages, invitations)
	}
	staff, err := store.CreateManagedUser(ctx, manager, domain.CreateUserInput{Username: "cashier.shared", FullName: "Kasir Bersama", Role: domain.RoleAdmin}, "temporary-password-hash")
	if err != nil {
		t.Fatal(err)
	}
	if !staff.MustChangePassword || !staff.IsActive {
		t.Fatalf("new account state: %+v", staff)
	}
	account, err := store.CreateAccountSessionWithProtocol(ctx, staff.ID, integrationBytes(221), 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.SwitchContextSession(ctx, account, integrationBytes(221), integrationBytes(222), domain.SwitchContextInput{Kind: domain.ContextTenant, TenantID: tenant.ID}); !domain.IsCode(err, domain.CodeUnauthorized) {
		t.Fatalf("temporary password bypass: %v", err)
	}
	if err = store.ChangeOwnPassword(ctx, account, "changed-password-hash"); err != nil {
		t.Fatal(err)
	}
	contexts, err := store.AvailableContexts(ctx, staff.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(contexts.Tenants) != 2 || contexts.CanManageOrganization || contexts.PlatformAdmin {
		t.Fatalf("all tenants unavailable: %+v", contexts)
	}
	account, err = store.PrincipalBySession(ctx, account.SessionID, integrationBytes(221))
	if err != nil {
		t.Fatal(err)
	}
	selected, err := store.SwitchContextSession(ctx, account, integrationBytes(221), integrationBytes(222), domain.SwitchContextInput{Kind: domain.ContextTenant, TenantID: tenant.ID})
	if err != nil {
		t.Fatal(err)
	}
	if selected.Role != domain.RoleAdmin || selected.MembershipID == uuid.Nil || selected.TenantID != tenant.ID {
		t.Fatalf("selection: %+v", selected)
	}
	// Legacy membership roles/statuses no longer change account authority.
	if _, err = store.Pool.Exec(ctx, `UPDATE tenant_memberships SET role='superadmin',status='inactive' WHERE id=$1`, selected.MembershipID); err != nil {
		t.Fatal(err)
	}
	selected, err = store.PrincipalBySession(ctx, selected.SessionID, integrationBytes(222))
	if err != nil || selected.Role != domain.RoleAdmin {
		t.Fatalf("membership still authoritative: %+v %v", selected, err)
	}
	if _, err = store.ListManagedUsers(ctx, selected); !domain.IsCode(err, domain.CodeForbidden) {
		t.Fatalf("business context accessed management: %v", err)
	}
	if _, err = store.CreatePackage(ctx, selected, domain.CreatePackageInput{Code: "FORBIDDEN", Name: "No", UnitPrice: 1000}); !domain.IsCode(err, domain.CodeForbidden) {
		t.Fatalf("Admin managed catalog: %v", err)
	}
	changed, err := store.UpdateManagedTenant(ctx, manager, tenant.ID, domain.UpdateTenantInput{Name: "Kios Baru", ExpectedRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if changed.Revision != 2 || changed.Slug != tenant.Slug {
		t.Fatalf("rename mutated identity: %+v", changed)
	}
	profile, err := store.GetTenantProfile(ctx, tenant.ID)
	if err != nil || profile.BusinessName != "Kios Telomoyo" {
		t.Fatalf("management rename changed receipt: %+v %v", profile, err)
	}
	if _, err = store.UpdateManagedTenant(ctx, manager, tenant.ID, domain.UpdateTenantInput{Name: "Stale", ExpectedRevision: 1}); !domain.IsCode(err, domain.CodeRevisionConflict) {
		t.Fatalf("stale rename accepted: %v", err)
	}
	// Suspension hides business and revokes its sessions, without deleting it.
	if _, err = store.SetTenantStatus(ctx, manager, tenant.ID, "suspended"); err != nil {
		t.Fatal(err)
	}
	contexts, err = store.AvailableContexts(ctx, staff.ID)
	if err != nil || len(contexts.Tenants) != 1 {
		t.Fatalf("suspended selector: %+v %v", contexts, err)
	}
	if _, err = store.SetTenantStatus(ctx, manager, tenant.ID, "active"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PrincipalBySession(ctx, selected.SessionID, integrationBytes(222)); err == nil {
		t.Fatal("reactivation revived old session")
	}
	// A role change affects every tenant and does not require membership grants.
	role := domain.RoleSuperadmin
	if _, err = store.UpdateManagedUser(ctx, manager, staff.ID, domain.UpdateManagedUserInput{Role: &role}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PrincipalBySession(ctx, selected.SessionID, integrationBytes(222)); err == nil {
		t.Fatal("role change revived session")
	}
	fresh, err := store.CreateAccountSessionWithProtocol(ctx, staff.ID, integrationBytes(223), 3)
	if err != nil || fresh.Role != role {
		t.Fatalf("new global role unavailable: %+v %v", fresh, err)
	}
	if _, err = store.ListManagedUsers(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	admin := domain.RoleAdmin
	disabled := false
	if _, err = store.UpdateManagedUser(ctx, manager, manager.UserID, domain.UpdateManagedUserInput{Role: &admin}); !domain.IsCode(err, domain.CodeSelfMutation) {
		t.Fatalf("self demotion: %v", err)
	}
	if _, err = store.UpdateManagedUser(ctx, manager, manager.UserID, domain.UpdateManagedUserInput{Active: &disabled}); !domain.IsCode(err, domain.CodeSelfMutation) {
		t.Fatalf("self deactivation: %v", err)
	}
	if _, err = store.UpdateManagedUser(ctx, manager, staff.ID, domain.UpdateManagedUserInput{Active: &disabled}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PrincipalBySession(ctx, fresh.SessionID, integrationBytes(223)); !domain.IsCode(err, domain.CodeAccountAccessChanged) {
		t.Fatalf("deactivated account not identified for quarantine: %v", err)
	}
	enabled := true
	if _, err = store.UpdateManagedUser(ctx, manager, staff.ID, domain.UpdateManagedUserInput{Active: &enabled}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PrincipalBySession(ctx, fresh.SessionID, integrationBytes(223)); !domain.IsCode(err, domain.CodeAccountAccessChanged) {
		t.Fatalf("reactivated account revived session: %v", err)
	}
	fresh, err = store.CreateAccountSessionWithProtocol(ctx, staff.ID, integrationBytes(224), 3)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := store.ResetManagedPassword(ctx, manager, staff.ID, "new-temporary-password-hash")
	if err != nil || !recovered.MustChangePassword {
		t.Fatalf("recovery: %+v %v", recovered, err)
	}
	if _, err = store.PrincipalBySession(ctx, fresh.SessionID, integrationBytes(224)); !domain.IsCode(err, domain.CodeAccountAccessChanged) {
		t.Fatalf("password recovery session still active: %v", err)
	}
	var leaked bool
	if err = store.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM platform_audit_events WHERE metadata::text LIKE '%password-hash%')`).Scan(&leaked); err != nil || leaked {
		t.Fatalf("credential leaked to audit: %v %v", leaked, err)
	}
}

func TestConcurrentGlobalSuperadminDemotionCannotRemoveLastManager(t *testing.T) {
	store := openSandboxLifecycleStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	owner, _ := seedSandboxLifecyclePrincipal(t, ctx, store)
	a, err := store.CreateAccountSessionWithProtocol(ctx, owner.UserID, integrationBytes(225), 3)
	if err != nil {
		t.Fatal(err)
	}
	bUser, err := store.CreateManagedUser(ctx, a, domain.CreateUserInput{Username: "second.superadmin", FullName: "Second", Role: domain.RoleSuperadmin}, "hash")
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.CreateAccountSessionWithProtocol(ctx, bUser.ID, integrationBytes(226), 3)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.ChangeOwnPassword(ctx, b, "changed"); err != nil {
		t.Fatal(err)
	}
	b, err = store.PrincipalBySession(ctx, b.SessionID, integrationBytes(226))
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	role := domain.RoleAdmin
	for _, pair := range [][2]domain.Principal{{a, b}, {b, a}} {
		go func(actor, target domain.Principal) {
			<-start
			_, e := store.UpdateManagedUser(ctx, actor, target.UserID, domain.UpdateManagedUserInput{Role: &role})
			results <- e
		}(pair[0], pair[1])
	}
	close(start)
	succeeded := 0
	for range 2 {
		if e := <-results; e == nil {
			succeeded++
		} else if !domain.IsCode(e, domain.CodeForbidden) && !domain.IsCode(e, domain.CodeUnauthorized) {
			t.Fatalf("unexpected race failure: %v", e)
		}
	}
	if succeeded != 1 {
		t.Fatalf("mutually racing demotions succeeded %d times", succeeded)
	}
	var remaining int
	if err = store.Pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE role='superadmin' AND is_active AND deleted_at IS NULL`).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("remaining Superadmins=%d %v", remaining, err)
	}
}
