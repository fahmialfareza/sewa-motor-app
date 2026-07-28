package postgres

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/google/uuid"
)

func TestTransactionSnapshotMatchesPublicConflictContract(t *testing.T) {
	occurredAt := time.Date(2026, 7, 24, 1, 2, 3, 0, time.UTC)
	snapshot := transactionSnapshot(occurredAt, []domain.TransactionItem{{
		LineNumber: 1, PackageID: uuid.MustParse("00000000-0000-4000-8000-000000000001"),
		PackageRevision: 2, PackageCode: "STANDARD", PackageName: "Paket Standar",
		PackageDescription: "Deskripsi", UnitPrice: 70_000, Quantity: 2, LineTotal: 140_000,
	}}, 140_000)
	body, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"occurredAt", "items", "subtotal", "total"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("public snapshot is missing %q: %s", key, body)
		}
	}
	if len(decoded) != 4 {
		t.Fatalf("public snapshot has unexpected fields: %s", body)
	}
	items := decoded["items"].([]any)
	item := items[0].(map[string]any)
	for _, key := range []string{
		"packageId", "packageRevision", "name", "description", "unitPrice", "quantity", "lineTotal",
	} {
		if _, ok := item[key]; !ok {
			t.Fatalf("public item is missing %q: %s", key, body)
		}
	}
	if _, leaked := item["packageCode"]; leaked {
		t.Fatalf("internal packageCode leaked into public snapshot: %s", body)
	}
}
