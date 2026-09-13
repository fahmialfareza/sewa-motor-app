package postgres

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/adapter/httpapi"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/adapter/security"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/usecase"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Exercise the HTTP boundary as well as the repository: returning a valid
// tenant_metadata database event previously failed only when syncPull mapped it
// into its public payload after otherwise successful terminal enrollment.
func TestTerminalEnrollmentThenInitialHTTPPullIncludesTenantMetadata(t *testing.T) {
	store := openSandboxLifecycleStore(t)
	previousGinMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousGinMode) })
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	owner, _ := seedSandboxLifecyclePrincipal(t, ctx, store)
	tokens := security.OpaqueTokenManager{}
	_, managerHash, err := tokens.New()
	if err != nil {
		t.Fatal(err)
	}
	manager, err := store.CreateAccountSessionWithProtocol(ctx, owner.UserID, managerHash, 3)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateTenant(ctx, manager, domain.CreateTenantInput{Name: "Kios Uji Sinkron", Slug: "kios-uji-sinkron"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	router := httpapi.New(httpapi.Dependencies{
		Repo:      store,
		Auth:      usecase.Auth{Repo: store, Tokens: tokens, Sessions: &integrationLoginIndex{entries: map[string]uuid.UUID{}}},
		Terminals: usecase.Terminals{Repo: store},
		Sync:      usecase.Sync{Repo: store},
		Sandbox:   usecase.Sandbox{Repo: store, Enabled: true},
	})
	for _, tenantID := range []uuid.UUID{domain.InitialTenantID(), created.Tenant.ID} {
		t.Run(tenantID.String(), func(t *testing.T) {
			_, accountHash, err := tokens.New()
			if err != nil {
				t.Fatal(err)
			}
			account, err := store.CreateAccountSessionWithProtocol(ctx, owner.UserID, accountHash, 3)
			if err != nil {
				t.Fatal(err)
			}
			productionToken, productionHash, err := tokens.New()
			if err != nil {
				t.Fatal(err)
			}
			production, err := store.SwitchContextSession(ctx, account, accountHash, productionHash, domain.SwitchContextInput{Kind: domain.ContextTenant, TenantID: tenantID})
			if err != nil {
				t.Fatal(err)
			}
			if production.TerminalID != nil || production.DataMode != domain.DataModeProduction {
				t.Fatalf("expected unbound Production session: %+v", production)
			}
			publicKey, _, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			body, err := json.Marshal(map[string]any{
				"installationId": uuid.NewString(), "name": "MPOS Uji Sinkron", "algorithm": "Ed25519",
				"publicKey": base64.StdEncoding.EncodeToString(publicKey),
			})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/terminals/enroll", bytes.NewReader(body)).WithContext(ctx)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+productionToken)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusCreated {
				t.Fatalf("enroll status=%d body=%s", response.Code, response.Body.String())
			}
			production, err = store.PrincipalBySession(ctx, production.SessionID, productionHash)
			if err != nil || production.TerminalID == nil {
				t.Fatalf("enrollment did not bind session: %+v error=%v", production, err)
			}
			assertInitialHTTPMetadataPull(t, ctx, store, router, production, productionToken)

			// Both activation paths seed metadata too: a new tenant's Production
			// catalog and each tenant's first Sandbox generation must be pullable.
			sandbox, err := store.EnsureSandbox(ctx, tenantID)
			if err != nil {
				t.Fatal(err)
			}
			sandboxToken, sandboxHash, err := tokens.New()
			if err != nil {
				t.Fatal(err)
			}
			sandboxSession, err := store.SwitchSession(ctx, production, productionHash, sandboxHash, sandbox.ID)
			if err != nil {
				t.Fatal(err)
			}
			assertInitialHTTPMetadataPull(t, ctx, store, router, sandboxSession, sandboxToken)
		})
	}
}

func assertInitialHTTPMetadataPull(t *testing.T, ctx context.Context, store *Store, router http.Handler, principal domain.Principal, token string) {
	t.Helper()
	var expectedBody []byte
	if err := store.Pool.QueryRow(ctx, `SELECT payload FROM sync_changes WHERE data_space_id=$1 AND tenant_id=$2 AND aggregate='tenant_metadata' ORDER BY cursor LIMIT 1`, principal.DataSpaceID, principal.TenantID).Scan(&expectedBody); err != nil {
		t.Fatalf("metadata was not seeded: %v", err)
	}
	var expected any
	if err := json.Unmarshal(expectedBody, &expected); err != nil {
		t.Fatal(err)
	}
	cursor := ""
	var previous int64
	foundMetadata, foundTerminal := false, false
	for page := 0; page < 100; page++ {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/sync/pull?limit=1&cursor="+url.QueryEscape(cursor), nil).WithContext(ctx)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("initial %s pull status=%d body=%s", principal.DataMode, response.Code, response.Body.String())
		}
		var envelope struct {
			Data struct {
				Changes []struct {
					Cursor      string `json:"cursor"`
					Aggregate   string `json:"aggregate"`
					AggregateID string `json:"aggregateId"`
					Payload     any    `json:"payload"`
				} `json:"changes"`
				Cursor  string `json:"cursor"`
				HasMore bool   `json:"hasMore"`
			} `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		for _, change := range envelope.Data.Changes {
			next, err := domain.DecodeTenantSyncCursor(principal, change.Cursor)
			if err != nil || next <= previous {
				t.Fatalf("cursor did not progress within selected tenant/space: %q previous=%d error=%v", change.Cursor, previous, err)
			}
			var tenantID, spaceID uuid.UUID
			if err = store.Pool.QueryRow(ctx, `SELECT tenant_id,data_space_id FROM sync_changes WHERE cursor=$1`, next).Scan(&tenantID, &spaceID); err != nil {
				t.Fatal(err)
			}
			if tenantID != principal.TenantID || spaceID != principal.DataSpaceID {
				t.Fatalf("pull crossed scope: tenant=%s space=%s", tenantID, spaceID)
			}
			previous = next
			switch change.Aggregate {
			case "tenant_metadata":
				if change.AggregateID != principal.TenantID.String() || !reflect.DeepEqual(change.Payload, expected) {
					t.Fatalf("metadata payload changed or crossed tenant: %+v want=%+v", change, expected)
				}
				foundMetadata = true
			case "terminal":
				foundTerminal = foundTerminal || (principal.TerminalID != nil && change.AggregateID == principal.TerminalID.String())
			}
		}
		decoded, err := domain.DecodeTenantSyncCursor(principal, envelope.Data.Cursor)
		if err != nil || decoded != previous {
			t.Fatalf("page cursor does not match last change: %q previous=%d error=%v", envelope.Data.Cursor, previous, err)
		}
		cursor = envelope.Data.Cursor
		if !envelope.Data.HasMore {
			if !foundMetadata || !foundTerminal {
				t.Fatalf("initial pull missing metadata=%v or enrolled terminal=%v", foundMetadata, foundTerminal)
			}
			return
		}
	}
	t.Fatal("initial pull did not terminate")
}
