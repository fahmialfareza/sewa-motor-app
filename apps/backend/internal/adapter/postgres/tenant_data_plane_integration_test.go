package postgres

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/google/uuid"
)

// Global authority does not collapse independent tenant data and enrollments.
// Exercise business repositories, not only SQL constraints.
func TestTenantDataPlaneIsolationAndGlobalAuthority(t *testing.T) {
	store := openSandboxLifecycleStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	a, _ := seedSandboxLifecyclePrincipal(t, ctx, store)
	b := seedSecondTenantPrincipal(t, ctx, store, a.UserID, domain.RoleSuperadmin)
	now := time.Now().UTC()
	pa, err := store.CreatePackage(ctx, a, domain.CreatePackageInput{Code: "SAME", Name: "Tenant A", UnitPrice: 25000})
	if err != nil {
		t.Fatal(err)
	}
	pb, err := store.CreatePackage(ctx, b, domain.CreatePackageInput{Code: "SAME", Name: "Tenant B", UnitPrice: 40000})
	if err != nil {
		t.Fatal(err)
	}
	assertPackageNotFound(t, ctx, store, a.DataSpaceID, pb.ID)
	assertPackageNotFound(t, ctx, store, b.DataSpaceID, pa.ID)
	if _, err = store.GetTerminal(ctx, b.TenantID, *a.TerminalID); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("cross tenant terminal: %v", err)
	}
	if _, err = store.TerminalPublicKey(ctx, b.TenantID, *a.TerminalID); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("cross tenant key: %v", err)
	}
	if terminal, err := store.TerminalIDByInstallation(ctx, b.TenantID, uuid.MustParse(a.Terminal.InstallationID)); err != nil || terminal != nil {
		t.Fatalf("cross tenant installation: %v %v", terminal, err)
	}
	if _, err = store.DataSpaceByID(ctx, b.TenantID, a.DataSpaceID); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("cross tenant space: %v", err)
	}
	if _, err = store.GetUser(ctx, b.TenantID, uuid.New()); !domain.IsCode(err, domain.CodeNotFound) {
		t.Fatalf("cross tenant user projection: %v", err)
	}

	create := func(actor domain.Principal, p domain.Package, id string) domain.Transaction {
		tr, err := store.CreateTransaction(ctx, domain.CreateTransactionInput{ID: id, OccurredAt: now, PaymentMethod: domain.PaymentMethodCash, Items: []domain.ItemInput{{PackageID: p.ID, PackageRevision: 1, Quantity: 1}}, Identity: principalIdentity(actor)})
		if err != nil {
			t.Fatal(err)
		}
		return tr
	}
	ta := create(a, pa, "01ARZ3NDEKTSV4RRFFQ69G5FA1")
	tb := create(b, pb, "01ARZ3NDEKTSV4RRFFQ69G5FA2")
	assertTransactionNotFound(t, ctx, store, a.DataSpaceID, tb.ID)
	assertTransactionNotFound(t, ctx, store, b.DataSpaceID, ta.ID)
	assertTransactionList(t, ctx, store, a.DataSpaceID, ta.ID)
	assertTransactionList(t, ctx, store, b.DataSpaceID, tb.ID)
	assertCrossSpaceTransactionMutationsRejected(t, ctx, store, a, b.DataSpaceID, tb.ID, pa, now)
	assertCrossSpaceTransactionMutationsRejected(t, ctx, store, b, a.DataSpaceID, ta.ID, pb, now)
	if tb.ReceiptIdentity == nil || tb.ReceiptIdentity.BusinessName != "Usaha Kedua" {
		t.Fatalf("wrong merchant receipt: %+v", tb.ReceiptIdentity)
	}

	// Historical membership role/status must no longer authorize or deny access.
	if _, err = store.Pool.Exec(ctx, `UPDATE tenant_memberships SET role='admin',status='inactive' WHERE id=$1`, b.MembershipID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.UpdatePackage(ctx, b, pb.ID, domain.UpdatePackageInput{Name: "Global owner", UnitPrice: 40000, ChangeReason: "global role"}); err != nil {
		t.Fatalf("historical membership blocked global Superadmin: %v", err)
	}
	// Read the global role again inside the mutation transaction, even when both
	// cached principals still advertise Superadmin.
	if _, err = store.Pool.Exec(ctx, `UPDATE users SET role='admin' WHERE id=$1`, a.UserID); err != nil {
		t.Fatal(err)
	}
	for _, pair := range []struct {
		actor domain.Principal
		pack  domain.Package
	}{{a, pa}, {b, pb}} {
		if _, err = store.UpdatePackage(ctx, pair.actor, pair.pack.ID, domain.UpdatePackageInput{Name: "Forbidden", UnitPrice: 1, ChangeReason: "stale role"}); !domain.IsCode(err, domain.CodeForbidden) {
			t.Fatalf("stale global role authorized tenant %s: %v", pair.actor.TenantID, err)
		}
	}
	if _, err = store.Pool.Exec(ctx, `UPDATE users SET role='superadmin' WHERE id=$1`, a.UserID); err != nil {
		t.Fatal(err)
	}
	confirmPayment(t, ctx, store, b, tb.ID, 1, now)
	confirmPayment(t, ctx, store, a, ta.ID, 1, now)
	for _, pair := range []struct {
		actor  domain.Principal
		amount int64
	}{{a, 25000}, {b, 40000}} {
		dashboard, err := store.Dashboard(ctx, pair.actor.DataSpaceID, now.Add(-time.Hour), now.Add(time.Hour), "day")
		if err != nil || dashboard.GrossRevenue != pair.amount || dashboard.TransactionCount != 1 {
			t.Fatalf("isolated dashboard: %+v %v", dashboard, err)
		}
		rows, err := store.ExportRows(ctx, domain.TransactionFilter{DataSpaceID: pair.actor.DataSpaceID})
		if err != nil || len(rows) != 1 || rows[0].TransactionTotal != pair.amount {
			t.Fatalf("isolated export: %+v %v", rows, err)
		}
	}

	// New tenants cannot use an unknown merchant payload merely because the
	// same global user enrolled it in another business.
	hash := strings.Repeat("a", 64)
	if _, err = store.Pool.Exec(ctx, `INSERT INTO tenant_qris_revisions(tenant_id,revision,payload_hash,static_payload,created_by) VALUES($1,1,$2,'fixture',$3)`, a.TenantID, hash, a.UserID); err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateTransaction(ctx, domain.CreateTransactionInput{ID: "01ARZ3NDEKTSV4RRFFQ69G5FA3", OccurredAt: now, PaymentMethod: domain.PaymentMethodQRIS, QrisPayloadHash: &hash, Items: []domain.ItemInput{{PackageID: pb.ID, PackageRevision: 1, Quantity: 1}}, Identity: principalIdentity(b)})
	if !domain.IsCode(err, domain.CodeValidation) {
		t.Fatalf("cross tenant merchant accepted: %v", err)
	}

	// Isolated Sandbox creation clones only its own production packages and
	// its own profile/merchant config, even with equal package codes.
	sa, err := store.EnsureSandbox(ctx, a.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	sb, err := store.EnsureSandbox(ctx, b.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	clones, err := store.ListPackages(ctx, sb.ID, false)
	if err != nil || len(clones) != 1 || clones[0].SourcePackageID == nil || *clones[0].SourcePackageID != pb.ID {
		t.Fatalf("foreign Sandbox clone: %+v %v", clones, err)
	}
	if _, err = store.ResetSandbox(ctx, a, sa.Generation, 30*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	activeB, err := store.ActiveDataSpace(ctx, b.TenantID, domain.DataModeSandbox)
	if err != nil || activeB.ID != sb.ID {
		t.Fatalf("reset crossed tenant: %+v %v", activeB, err)
	}
	assertOriginSessionScope(t, ctx, store, a, *a.TerminalID, b.DataSpaceID, false)

	// Missing scope cannot turn into the original tenant.
	if packages, err := store.ListPackages(ctx, uuid.Nil, false); err == nil && len(packages) > 0 {
		t.Fatal("missing scope disclosed packages")
	}
	if _, err = store.CreatePackage(ctx, domain.Principal{}, domain.CreatePackageInput{Code: "UNSCOPED", Name: "bad", UnitPrice: 1}); !domain.IsCode(err, domain.CodeContextRequired) {
		t.Fatalf("unscoped mutation: %v", err)
	}
	if _, err = store.Pool.Exec(ctx, `UPDATE tenants SET status='suspended' WHERE id=$1`, b.TenantID); err != nil {
		t.Fatal(err)
	}
	_, err = store.SetTransactionPaymentStatus(ctx, domain.SetPaymentStatusInput{ID: tb.ID, BaseRevision: 1, Status: domain.PaymentStatusFailed, OccurredAt: now, Identity: principalIdentity(b)})
	if !domain.IsCode(err, domain.CodeTenantSuspended) {
		t.Fatalf("suspended tenant write: %v", err)
	}
	if _, err = store.GetTransaction(ctx, a.DataSpaceID, ta.ID, true); err != nil {
		t.Fatalf("other tenant affected by suspension: %v", err)
	}
}

func seedSecondTenantPrincipal(t *testing.T, ctx context.Context, store *Store, userID uuid.UUID, role domain.Role) domain.Principal {
	t.Helper()
	tenantID, spaceID, membershipID, terminalID, sessionID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenants(id,name,slug,status) VALUES($1,'Usaha Kedua',$2,'active')`, []any{tenantID, "second-" + tenantID.String()}},
		{`INSERT INTO tenant_profile_revisions(tenant_id,revision,business_name,address,phone,created_by) VALUES($1,1,'Usaha Kedua','Alamat Kedua','08123',$2)`, []any{tenantID, userID}},
		{`INSERT INTO tenant_memberships(id,tenant_id,user_id,role,status) VALUES($1,$2,$3,$4,'active')`, []any{membershipID, tenantID, userID, role}},
		{`INSERT INTO data_spaces(id,tenant_id,mode,generation,status) VALUES($1,$2,'production',1,'active')`, []any{spaceID, tenantID}},
		{`INSERT INTO terminals(id,tenant_id,installation_id,name,public_key,enrolled_by) VALUES($1,$2,$3,'Second terminal',$4,$5)`, []any{terminalID, tenantID, uuid.New(), integrationBytes(71), userID}},
		{`INSERT INTO sessions(id,user_id,terminal_id,token_hash,data_space_id) VALUES($1,$2,$3,$4,$5)`, []any{sessionID, userID, terminalID, integrationBytes(72), spaceID}},
	} {
		if _, err = tx.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	principal, err := store.PrincipalBySession(ctx, sessionID, integrationBytes(72))
	if err != nil {
		t.Fatal(err)
	}
	return principal
}

func TestTenantChangeFanoutRebuildsGlobalAccountProjection(t *testing.T) {
	store := openSandboxLifecycleStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	a, _ := seedSandboxLifecyclePrincipal(t, ctx, store)
	b := seedSecondTenantPrincipal(t, ctx, store, a.UserID, domain.RoleAdmin)
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = addTenantChange(ctx, tx, b.TenantID, "user", a.UserID.String(), "updated", nil, map[string]any{"role": "admin", "passwordHash": "do not copy"}, false); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = store.Pool.QueryRow(ctx, `SELECT count(*) FROM sync_changes WHERE aggregate='user' AND aggregate_id=$1 AND data_space_id=$2`, a.UserID.String(), a.DataSpaceID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("membership change leaked to first tenant: %d %v", count, err)
	}
	var payload json.RawMessage
	if err = store.Pool.QueryRow(ctx, `SELECT payload FROM sync_changes WHERE aggregate='user' AND aggregate_id=$1 AND data_space_id=$2`, a.UserID.String(), b.DataSpaceID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var projected map[string]any
	if err = json.Unmarshal(payload, &projected); err != nil {
		t.Fatal(err)
	}
	if projected["role"] != "superadmin" || projected["membershipId"] != b.MembershipID.String() || projected["passwordHash"] != nil {
		t.Fatalf("unsafe membership projection: %s", payload)
	}
	tx, err = store.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE tenant_memberships SET status='inactive' WHERE id=$1`, b.MembershipID); err != nil {
		t.Fatal(err)
	}
	if err = addSharedChange(ctx, tx, "user", a.UserID.String(), "updated", nil, map[string]any{"fullName": "ignored"}, false); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var action string
	var tombstone bool
	if err = store.Pool.QueryRow(ctx, `SELECT action,tombstone FROM sync_changes WHERE data_space_id=$1 AND aggregate='user' ORDER BY cursor DESC LIMIT 1`, b.DataSpaceID).Scan(&action, &tombstone); err != nil {
		t.Fatal(err)
	}
	if action != "updated" || tombstone {
		t.Fatalf("historical membership status hid global account: action=%s tombstone=%v", action, tombstone)
	}
	tx, err = store.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE users SET is_active=false WHERE id=$1`, a.UserID); err != nil {
		t.Fatal(err)
	}
	if err = addSharedChange(ctx, tx, "user", a.UserID.String(), "updated", nil, nil, false); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{a.DataSpaceID, b.DataSpaceID} {
		if err = store.Pool.QueryRow(ctx, `SELECT action,tombstone FROM sync_changes WHERE data_space_id=$1 AND aggregate='user' ORDER BY cursor DESC LIMIT 1`, id).Scan(&action, &tombstone); err != nil {
			t.Fatal(err)
		}
		if action != "deleted" || !tombstone {
			t.Fatalf("inactive account remains visible in %s", id)
		}
	}
}
