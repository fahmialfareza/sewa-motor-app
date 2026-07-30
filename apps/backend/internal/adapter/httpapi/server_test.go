package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/port"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type healthRepository struct {
	port.Repository
}

type syncViewRepository struct {
	port.Repository
}

type transactionViewRepository struct {
	port.Repository
}

func (transactionViewRepository) ListPrintAttempts(context.Context, string) ([]domain.PrintAttempt, error) {
	return nil, nil
}

func (syncViewRepository) GetPackage(context.Context, uuid.UUID) (domain.Package, error) {
	return domain.Package{
		ID:   uuid.MustParse("00000000-0000-4000-8000-000000000001"),
		Code: "STANDARD", CurrentRevision: 3, Name: "Paket Standar",
		Description: "Deskripsi", UnitPrice: 80_000,
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC),
	}, nil
}

func TestSyncPackagePayloadUsesPublicDTO(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/sync/pull", nil)
	contextRecorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(contextRecorder)
	c.Request = request
	server := &Server{deps: Dependencies{Repo: syncViewRepository{}}}
	payload, err := server.syncChangePayload(c, domain.SyncChange{
		Aggregate: "package", AggregateID: "00000000-0000-4000-8000-000000000001",
	})
	if err != nil {
		t.Fatal(err)
	}
	view, ok := payload.(gin.H)
	if !ok {
		t.Fatalf("payload has type %T", payload)
	}
	if _, ok := view["currentRevision"].(gin.H); !ok {
		t.Fatalf("public currentRevision missing: %+v", view)
	}
	if _, leaked := view["revision"]; leaked {
		t.Fatalf("internal revision leaked: %+v", view)
	}
}

func TestTransactionViewIncludesPaymentConfirmation(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/transactions/example", nil)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = request
	server := &Server{deps: Dependencies{Repo: transactionViewRepository{}}}
	confirmedRevision := 2
	qrisPayloadHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	occurredAt := time.Date(
		2026, 7, 29, 18, 0, 0, 0,
		time.FixedZone("WIB", 7*60*60),
	)

	view, err := server.transactionView(c, domain.Transaction{
		ID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", Revision: 2,
		OccurredAt:               occurredAt,
		PaymentMethod:            domain.PaymentMethodQRIS,
		QrisPayloadHash:          &qrisPayloadHash,
		PaymentStatus:            domain.PaymentStatusSuccess,
		PaymentConfirmedRevision: &confirmedRevision,
		Items:                    []domain.TransactionItem{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if view["paymentMethod"] != domain.PaymentMethodQRIS ||
		view["qrisPayloadHash"] != qrisPayloadHash ||
		view["paymentStatus"] != domain.PaymentStatusSuccess ||
		view["paymentConfirmedRevision"] != &confirmedRevision {
		t.Fatalf("payment fields missing from transaction view: %+v", view)
	}
	if got, ok := view["occurredAt"].(time.Time); !ok ||
		got.Location() != time.UTC ||
		!got.Equal(occurredAt) {
		t.Fatalf("occurredAt was not serialized in UTC: %#v", view["occurredAt"])
	}
}

func TestRevisionConflictNormalizesLegacyIdempotencySnapshots(t *testing.T) {
	t.Parallel()

	legacySnapshot := map[string]any{
		"occurredAt": "2026-01-01T00:00:00Z",
		"items":      []any{},
		"subtotal":   0,
		"total":      0,
	}
	normalized := normalizeRevisionConflictDetails(map[string]any{
		"baseRevision":    2,
		"currentRevision": 4,
		"localSnapshot":   legacySnapshot,
		"serverSnapshot":  legacySnapshot,
	})
	details, ok := normalized.(map[string]any)
	if !ok {
		t.Fatalf("normalized details have type %T", normalized)
	}
	for name, revision := range map[string]float64{
		"localSnapshot":  3,
		"serverSnapshot": 4,
	} {
		snapshot, ok := details[name].(map[string]any)
		if !ok {
			t.Fatalf("%s has type %T", name, details[name])
		}
		if snapshot["paymentMethod"] != string(domain.PaymentMethodLegacy) ||
			snapshot["paymentStatus"] != string(domain.PaymentStatusSuccess) ||
			snapshot["paymentConfirmedRevision"] != revision {
			t.Fatalf("%s was not normalized: %+v", name, snapshot)
		}
	}
}

func (healthRepository) Ping(context.Context) error { return nil }

func TestLivenessEnvelopeAndRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := New(Dependencies{Repo: healthRepository{}})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body)
	}
	if _, err := uuid.Parse(recorder.Header().Get("X-Request-Id")); err != nil {
		t.Fatalf("invalid request ID: %v", err)
	}
	var envelope struct {
		Data struct {
			Status   string `json:"status"`
			Postgres string `json:"postgres"`
			Redis    string `json:"redis"`
		} `json:"data"`
		Meta map[string]any `json:"meta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Status != "ok" || envelope.Data.Postgres != "ok" || envelope.Data.Redis != "unavailable" {
		t.Fatalf("unexpected health envelope: %+v", envelope.Data)
	}
	if envelope.Meta["requestId"] == nil || envelope.Meta["serverTime"] == nil {
		t.Fatalf("missing metadata: %+v", envelope.Meta)
	}
}
