package usecase

import (
	"context"
	"testing"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/port"
	"github.com/google/uuid"
)

type sessionPolicyRepository struct {
	port.Repository
	upgraded         domain.Principal
	called           bool
	current          domain.Principal
	oldHash, newHash []byte
	protocol         int
	err              error
}

func (repository *sessionPolicyRepository) CreateAccountSessionWithProtocol(context.Context, uuid.UUID, []byte, int) (domain.Principal, error) {
	panic("not used by upgrade tests")
}

func (repository *sessionPolicyRepository) UpgradeSession(_ context.Context, current domain.Principal, oldHash, newHash []byte, protocol int) (domain.Principal, error) {
	repository.called = true
	repository.current = current
	repository.oldHash, repository.newHash, repository.protocol = oldHash, newHash, protocol
	return repository.upgraded, repository.err
}

func TestUpgradeSessionExchangesTokenWithoutChangingOriginPrincipal(t *testing.T) {
	old := domain.Principal{SessionID: uuid.New(), UserID: uuid.New(), ContextKind: domain.ContextTenant,
		TenantID: domain.InitialTenantID(), MembershipID: uuid.New(), DataSpaceID: uuid.New(),
		DataMode: domain.DataModeSandbox, SandboxGeneration: 9, ProtocolVersion: 2,
		SandboxQRISPolicy: domain.SandboxQRISPolicyFixed1000}
	upgraded := old
	upgraded.SessionID, upgraded.ProtocolVersion, upgraded.SandboxQRISPolicy = uuid.New(), 3, domain.SandboxQRISPolicyTransactionTotal
	repo := &sessionPolicyRepository{upgraded: upgraded}
	index := &memorySessionIndex{entries: map[string]uuid.UUID{"old-hash": old.SessionID}}
	service := Auth{Repo: repo, Sessions: index, Tokens: fixedAuthTokens{raw: "new-token", replacementHash: []byte("new-hash")}}
	result, err := service.UpgradeSession(context.Background(), Authentication{Principal: old, TokenHash: []byte("old-hash")}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !repo.called || repo.current != old || string(repo.oldHash) != "old-hash" || string(repo.newHash) != "new-hash" || repo.protocol != 3 {
		t.Fatalf("upgrade request did not retain original context: %+v", repo)
	}
	if result.Token != "new-token" || result.Principal != upgraded {
		t.Fatalf("unexpected upgrade result: %+v", result)
	}
	if _, exists := index.entries["old-hash"]; exists || index.entries["new-hash"] != upgraded.SessionID {
		t.Fatalf("session cache did not exchange token: %+v", index.entries)
	}
}

func TestUpgradeSessionRejectsRecoveryAndUnsupportedProtocolWithoutMutatingCache(t *testing.T) {
	for _, test := range []struct {
		name     string
		auth     Authentication
		protocol int
		code     string
	}{
		{"retired generation", Authentication{RecoverableRetiredSandbox: true}, 3, domain.CodeUnauthorized},
		{"revoked access", Authentication{RecoverableTenantAccess: true}, 3, domain.CodeUnauthorized},
		{"old protocol", Authentication{}, 2, domain.CodeValidation},
		{"future protocol", Authentication{}, 4, domain.CodeValidation},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &sessionPolicyRepository{}
			service := Auth{Repo: repo}
			_, err := service.UpgradeSession(context.Background(), test.auth, test.protocol)
			if !domain.IsCode(err, test.code) || repo.called {
				t.Fatalf("invalid upgrade reached repository: %v called=%v", err, repo.called)
			}
		})
	}
}

func TestUpgradeSessionFailurePreservesOldCachedSession(t *testing.T) {
	id := uuid.New()
	repo := &sessionPolicyRepository{err: domain.NewError(domain.CodeUnauthorized, "Sesi telah dicabut")}
	index := &memorySessionIndex{entries: map[string]uuid.UUID{"old-hash": id}}
	service := Auth{Repo: repo, Sessions: index, Tokens: fixedAuthTokens{replacementHash: []byte("new-hash")}}
	_, err := service.UpgradeSession(context.Background(), Authentication{Principal: domain.Principal{SessionID: id}, TokenHash: []byte("old-hash")}, 3)
	if !domain.IsCode(err, domain.CodeUnauthorized) || len(index.entries) != 1 || index.entries["old-hash"] != id {
		t.Fatalf("failed upgrade mutated cache: %v %+v", err, index.entries)
	}
}

func TestAuthenticatePreservesAccountAccessChangeIdentityForQuarantine(t *testing.T) {
	principal := domain.Principal{SessionID: uuid.New(), UserID: uuid.New(), ContextKind: domain.ContextTenant,
		TenantID: domain.InitialTenantID(), MembershipID: uuid.New(), DataSpaceID: uuid.New(), DataMode: domain.DataModeSandbox}
	repo := &authRecoveryRepository{retiredPrincipal: principal, retiredError: domain.NewError(domain.CodeAccountAccessChanged, "Akses akun telah berubah")}
	index := &memorySessionIndex{entries: map[string]uuid.UUID{}}
	service := Auth{Repo: repo, Tokens: fixedAuthTokens{hash: []byte("changed-hash")}, Sessions: index}
	authentication, err := service.Authenticate(context.Background(), "changed-token")
	if !domain.IsCode(err, domain.CodeAccountAccessChanged) || !authentication.RecoverableTenantAccess || authentication.Principal != principal || len(index.entries) != 0 {
		t.Fatalf("changed-access discovery lost scoped quarantine identity or cached invalid session: %+v %v", authentication, err)
	}
}
