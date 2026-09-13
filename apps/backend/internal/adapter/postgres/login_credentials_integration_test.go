package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/port"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/usecase"
	"github.com/google/uuid"
)

type interleavedLoginPasswords struct{ afterVerification func() }

func (p interleavedLoginPasswords) Hash(string) (string, error) { panic("not used during login") }
func (p interleavedLoginPasswords) Verify(password, hash string) (bool, error) {
	verified := password == hash
	if p.afterVerification != nil {
		p.afterVerification()
	}
	return verified, nil
}

type integrationLoginTokens struct{ hash []byte }

func (t integrationLoginTokens) New() (string, []byte, error) { return "test-login-token", t.hash, nil }
func (t integrationLoginTokens) Hash(string) ([]byte, error)  { return t.hash, nil }

type integrationLoginIndex struct{ entries map[string]uuid.UUID }

func (s *integrationLoginIndex) Get(_ context.Context, hash []byte) (uuid.UUID, bool) {
	id, ok := s.entries[string(hash)]
	return id, ok
}
func (s *integrationLoginIndex) Set(_ context.Context, hash []byte, id uuid.UUID) {
	s.entries[string(hash)] = id
}
func (s *integrationLoginIndex) Delete(_ context.Context, hash []byte) {
	delete(s.entries, string(hash))
}

type integrationLoginLimiter struct{}

func (integrationLoginLimiter) Allow(context.Context, string, int, time.Duration) (bool, error) {
	return true, nil
}

func TestPasswordRecoveryBetweenVerificationAndSessionCreationRejectsOldCredentials(t *testing.T) {
	for _, protocol := range []int{0, 2, 3} {
		t.Run(fmt.Sprint(protocol), func(t *testing.T) {
			store := openSandboxLifecycleStore(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			owner, _ := seedSandboxLifecyclePrincipal(t, ctx, store)
			manager, err := store.CreateAccountSessionWithProtocol(ctx, owner.UserID, integrationBytes(241), 3)
			if err != nil {
				t.Fatal(err)
			}
			staff, err := store.CreateManagedUser(ctx, manager, domain.CreateUserInput{Username: "login.race", FullName: "Kasir Uji Login", Role: domain.RoleAdmin}, "old-hash")
			if err != nil {
				t.Fatal(err)
			}
			prior, err := store.CreateAccountSessionWithProtocol(ctx, staff.ID, integrationBytes(242), 3)
			if err != nil {
				t.Fatal(err)
			}
			tokenHash := integrationBytes(243)
			index := &integrationLoginIndex{entries: map[string]uuid.UUID{}}
			auth := usecase.Auth{Repo: store, Tokens: integrationLoginTokens{hash: tokenHash}, Sessions: index, Limiter: integrationLoginLimiter{},
				Passwords: interleavedLoginPasswords{afterVerification: func() {
					// This is the exact former race window: verification has used
					// the old snapshot, while recovery commits before token issuance.
					if _, err := store.ResetManagedPassword(ctx, manager, staff.ID, "replacement-hash"); err != nil {
						t.Fatal(err)
					}
				}}}
			result, err := auth.Login(ctx, domain.LoginInput{Username: staff.Username, Password: "old-hash", ClientProtocolVersion: protocol})
			if !domain.IsCode(err, domain.CodeInvalidCredentials) || result.Token != "" || len(index.entries) != 0 {
				t.Fatalf("old verified credentials survived recovery: %v", err)
			}
			var count int
			if err = store.Pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE token_hash=$1`, tokenHash).Scan(&count); err != nil || count != 0 {
				t.Fatalf("rejected credential race persisted a session: count=%d err=%v", count, err)
			}
			if _, err = store.PrincipalBySession(ctx, prior.SessionID, integrationBytes(242)); !domain.IsCode(err, domain.CodeAccountAccessChanged) {
				t.Fatalf("password recovery did not revoke existing account sessions: %v", err)
			}
			auth.Passwords = interleavedLoginPasswords{}
			result, err = auth.Login(ctx, domain.LoginInput{Username: staff.Username, Password: "replacement-hash", ClientProtocolVersion: protocol})
			if err != nil || !result.Principal.MustChangePassword {
				t.Fatalf("recovered credential did not require password change: %v", err)
			}
			if protocol < 2 && (!result.Principal.IsTenantContext() || result.Principal.TenantID != domain.InitialTenantID() || result.Principal.DataSpaceID != domain.LiveDataSpaceID()) || protocol >= 2 && result.Principal.ContextKind != domain.ContextAccount {
				t.Fatalf("login changed legacy/account context behavior: %+v", result.Principal)
			}
			if result.Principal.EffectiveSandboxQRISPolicy() != domain.SandboxQRISPolicyForProtocol(protocol) {
				t.Fatal("credential-bound issuance changed the negotiated Sandbox policy")
			}
		})
	}
}

func TestCredentialSessionWaitsForAccountLockAndRereadsChangedHash(t *testing.T) {
	store := openSandboxLifecycleStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	owner, _ := seedSandboxLifecyclePrincipal(t, ctx, store)
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE users SET password_hash='new-hash' WHERE id=$1`, owner.UserID); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := store.CreateVerifiedLoginSession(ctx, port.VerifiedLoginSession{UserID: owner.UserID, VerifiedPasswordHash: "hash", TokenHash: integrationBytes(244), ProtocolVersion: 3})
		result <- err
	}()
	// Wait for PostgreSQL itself to report the blocked credential read instead
	// of relying on a sleep to arrange the race.
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		var blocked bool
		if err = store.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE pid<>pg_backend_pid() AND wait_event_type='Lock' AND query LIKE 'SELECT is_active AND deleted_at IS NULL,password_hash FROM users%')`).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case err := <-result:
			t.Fatalf("session creation bypassed account lock: %v", err)
		case <-deadline.C:
			t.Fatal("credential read never waited on the password-change account lock")
		case <-tick.C:
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if !domain.IsCode(err, domain.CodeInvalidCredentials) {
			t.Fatalf("credential read did not observe committed replacement hash: %v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestTerminalEnrollmentRevalidatesGlobalRoleNotClaimedOrMembershipRole(t *testing.T) {
	store := openSandboxLifecycleStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	owner, _ := seedSandboxLifecyclePrincipal(t, ctx, store)
	account, err := store.CreateAccountSessionWithProtocol(ctx, owner.UserID, integrationBytes(245), 3)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := store.SwitchContextSession(ctx, account, integrationBytes(245), integrationBytes(246), domain.SwitchContextInput{Kind: domain.ContextTenant, TenantID: owner.TenantID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool.Exec(ctx, `UPDATE users SET role='admin' WHERE id=$1`, owner.UserID); err != nil {
		t.Fatal(err)
	}
	input := domain.EnrollTerminalInput{InstallationID: uuid.NewString(), Name: "Terminal Global Role", PublicKey: integrationBytes(247)}
	if _, err = store.EnrollTerminal(ctx, selected, input); !domain.IsCode(err, domain.CodeForbidden) {
		t.Fatalf("stale Superadmin principal/historical membership enrolled terminal after global demotion: %v", err)
	}
	if _, err = store.Pool.Exec(ctx, `UPDATE users SET role='superadmin' WHERE id=$1`, owner.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool.Exec(ctx, `UPDATE tenant_memberships SET role='admin',status='inactive' WHERE id=$1`, selected.MembershipID); err != nil {
		t.Fatal(err)
	}
	terminal, err := store.EnrollTerminal(ctx, selected, input)
	if err != nil {
		t.Fatalf("historical membership blocked global Superadmin enrollment: %v", err)
	}
	var tenantID uuid.UUID
	if err = store.Pool.QueryRow(ctx, `SELECT tenant_id FROM terminals WHERE id=$1`, terminal.ID).Scan(&tenantID); err != nil || tenantID != owner.TenantID {
		t.Fatalf("enrollment did not retain selected tenant: tenant=%s err=%v", tenantID, err)
	}
}
