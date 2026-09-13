package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/port"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/usecase"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestSyncTenantConfigurationPayloadsPreserveStoredPublicFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, aggregate := range []string{"tenant_metadata", "tenant_profile", "tenant_qris"} {
		t.Run(aggregate, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/sync/pull", nil)
			server := &Server{}
			// These configuration snapshots are already tenant-scoped by the
			// repository. Preserve the stored public shape, including nulls.
			raw := json.RawMessage(`{"id":"00000000-0000-4000-8000-000000000200","name":"Telomoyo","revision":2,"qrisRevision":null}`)
			payload, err := server.syncChangePayload(c, domain.SyncChange{Aggregate: aggregate, Payload: raw})
			if err != nil {
				t.Fatal(err)
			}
			var expected any
			if err := json.Unmarshal(raw, &expected); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(payload, expected) {
				t.Fatalf("configuration payload changed: got %#v, want %#v", payload, expected)
			}
			if _, err := server.syncChangePayload(c, domain.SyncChange{Aggregate: aggregate, Payload: json.RawMessage(`{`)}); !domain.IsCode(err, domain.CodeInternal) {
				t.Fatalf("malformed stored configuration must fail instead of advancing sync: %v", err)
			}
		})
	}
}

type tenantMetadataPullRepository struct {
	port.Repository
	current domain.Principal
	changes []domain.SyncChange
	space   uuid.UUID
}

func (r *tenantMetadataPullRepository) PrincipalByTokenHash(context.Context, []byte) (domain.Principal, error) {
	return r.current, nil
}

func (r *tenantMetadataPullRepository) PrincipalBySession(context.Context, uuid.UUID, []byte) (domain.Principal, error) {
	return r.current, nil
}

func (r *tenantMetadataPullRepository) PullChanges(_ context.Context, space uuid.UUID, cursor int64, limit int) ([]domain.SyncChange, error) {
	r.space = space
	var changes []domain.SyncChange
	for _, change := range r.changes {
		if change.Cursor > cursor {
			changes = append(changes, change)
			if len(changes) == limit {
				break
			}
		}
	}
	return changes, nil
}

func TestSyncPullTenantMetadataSupportsInitialPullPaginationAndTombstones(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mode := range []domain.DataMode{domain.DataModeProduction, domain.DataModeSandbox} {
		t.Run(string(mode), func(t *testing.T) {
			terminalID := uuid.New()
			p := domain.Principal{
				ContextKind: domain.ContextTenant, TenantID: domain.InitialTenantID(), MembershipID: uuid.New(),
				UserID: uuid.New(), SessionID: uuid.New(), TerminalID: &terminalID, Role: domain.RoleSuperadmin,
				DataSpaceID: domain.LiveDataSpaceID(), DataMode: mode, ProtocolVersion: 3,
				SandboxQRISPolicy: domain.SandboxQRISPolicyTransactionTotal,
			}
			if mode == domain.DataModeSandbox {
				p.DataSpaceID = uuid.New()
				p.SandboxGeneration = 2
			}
			tenant := domain.Tenant{ID: p.TenantID, Name: "Telomoyo", Slug: "telomoyo", Status: "active", ProfileRevision: 1, Revision: 1}
			body, err := json.Marshal(tenant)
			if err != nil {
				t.Fatal(err)
			}
			repo := &tenantMetadataPullRepository{current: p, changes: []domain.SyncChange{
				{Cursor: 4, Aggregate: "tenant_metadata", AggregateID: p.TenantID.String(), Action: "upsert", Revision: &tenant.Revision, Payload: body, DataSpaceID: p.DataSpaceID, CreatedAt: time.Now().UTC()},
				{Cursor: 7, Aggregate: "tenant_metadata", AggregateID: p.TenantID.String(), Action: "delete", Tombstone: true, Payload: json.RawMessage(`invalid but unused tombstone body`), DataSpaceID: p.DataSpaceID, CreatedAt: time.Now().UTC()},
			}}
			router := New(Dependencies{
				Repo: repo, Sync: usecase.Sync{Repo: repo},
				Auth:    usecase.Auth{Repo: repo, Tokens: retiredRecoveryTokens{}, Sessions: &retiredRecoverySessionIndex{values: map[string]uuid.UUID{}}, SandboxEnabled: true},
				Sandbox: usecase.Sandbox{Enabled: true},
			})
			cursor := ""
			for index, expected := range repo.changes {
				request := httptest.NewRequest(http.MethodGet, "/api/v1/sync/pull?limit=1&cursor="+url.QueryEscape(cursor), nil)
				request.Header.Set("Authorization", "Bearer enrolled-session")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != http.StatusOK {
					t.Fatalf("initial/configuration pull failed: status=%d body=%s", response.Code, response.Body.String())
				}
				var envelope struct {
					Data struct {
						Changes []struct {
							Cursor      string          `json:"cursor"`
							Aggregate   string          `json:"aggregate"`
							AggregateID string          `json:"aggregateId"`
							Payload     json.RawMessage `json:"payload"`
							Tombstone   bool            `json:"tombstone"`
						} `json:"changes"`
						Cursor  string `json:"cursor"`
						HasMore bool   `json:"hasMore"`
					} `json:"data"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if len(envelope.Data.Changes) != 1 || envelope.Data.HasMore != (index == 0) || repo.space != p.DataSpaceID {
					t.Fatal("metadata pull lost pagination or authenticated data-space scope")
				}
				change := envelope.Data.Changes[0]
				wantCursor := domain.EncodeTenantSyncCursor(p, expected.Cursor)
				if change.Aggregate != "tenant_metadata" || change.AggregateID != p.TenantID.String() || change.Tombstone != expected.Tombstone || change.Cursor != wantCursor || envelope.Data.Cursor != wantCursor {
					t.Fatal("metadata event identity or scoped cursor changed")
				}
				if expected.Tombstone {
					if string(change.Payload) != "null" {
						t.Fatal("tombstone payload must remain null")
					}
				} else {
					var got domain.Tenant
					if err := json.Unmarshal(change.Payload, &got); err != nil || !reflect.DeepEqual(got, tenant) {
						t.Fatalf("initial pull lost tenant metadata: %v", err)
					}
				}
				cursor = envelope.Data.Cursor
			}
		})
	}
}

func TestSyncUnknownAggregateStillFailsClosed(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/sync/pull", nil)
	if _, err := (&Server{}).syncChangePayload(c, domain.SyncChange{Aggregate: "unknown_future_aggregate", Payload: json.RawMessage(`{}`)}); !domain.IsCode(err, domain.CodeInternal) {
		t.Fatalf("unknown aggregate must not be silently skipped: %v", err)
	}
}
