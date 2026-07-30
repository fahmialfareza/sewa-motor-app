package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fahmialfareza/sewa-motor-app/apps/backend/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestTransactionSnapshotMatchesPublicConflictContract(t *testing.T) {
	occurredAt := time.Date(2026, 7, 24, 1, 2, 3, 0, time.UTC)
	snapshot := paymentStateSnapshot(
		transactionSnapshot(occurredAt, domain.PaymentMethodCash, nil, []domain.TransactionItem{{
			LineNumber: 1, PackageID: uuid.MustParse("00000000-0000-4000-8000-000000000001"),
			PackageRevision: 2, PackageCode: "STANDARD", PackageName: "Paket Standar",
			PackageDescription: "Deskripsi", UnitPrice: 70_000, Quantity: 2, LineTotal: 140_000,
		}}, 140_000),
		domain.PaymentStatusPending,
		nil,
	)
	body, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"occurredAt", "paymentMethod", "paymentStatus",
		"paymentConfirmedRevision", "items", "subtotal", "total",
	} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("public snapshot is missing %q: %s", key, body)
		}
	}
	if len(decoded) != 7 {
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

func TestTransactionSnapshotIncludesQrisPayloadBinding(t *testing.T) {
	t.Parallel()

	hash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	snapshot := transactionSnapshot(
		time.Date(2026, 7, 30, 1, 2, 3, 0, time.UTC),
		domain.PaymentMethodQRIS,
		&hash,
		nil,
		70_000,
	)
	if snapshot["qrisPayloadHash"] != hash {
		t.Fatalf("QRIS payload hash missing from immutable snapshot: %#v", snapshot)
	}
}

type resolvedPackageQuery struct {
	unitPrice int64
}

func (query resolvedPackageQuery) QueryRow(
	_ context.Context,
	_ string,
	_ ...any,
) pgx.Row {
	return resolvedPackageRow{unitPrice: query.unitPrice}
}

type resolvedPackageRow struct {
	unitPrice int64
}

func (row resolvedPackageRow) Scan(destinations ...any) error {
	*destinations[0].(*uuid.UUID) = uuid.MustParse("00000000-0000-4000-8000-000000000001")
	*destinations[1].(*int) = 1
	*destinations[2].(*string) = "QRIS-LIMIT"
	*destinations[3].(*string) = "Paket Uji Batas QRIS"
	*destinations[4].(*string) = "Harga berasal dari snapshot paket server"
	*destinations[5].(*int64) = row.unitPrice
	return nil
}

func TestResolveTransactionItemsEnforcesQRISLimitAfterPriceResolution(t *testing.T) {
	t.Parallel()

	inputs := []domain.ItemInput{{
		PackageID:       uuid.MustParse("00000000-0000-4000-8000-000000000001"),
		PackageRevision: 1,
		Quantity:        1,
	}}
	query := resolvedPackageQuery{
		unitPrice: domain.MaxQRISTransactionTotal + 1,
	}

	if _, _, err := resolveTransactionItems(
		context.Background(),
		query,
		domain.PaymentMethodQRIS,
		inputs,
	); !domain.IsCode(err, domain.CodeValidation) {
		t.Fatalf("expected resolved QRIS total to be rejected, got %v", err)
	}

	items, total, err := resolveTransactionItems(
		context.Background(),
		query,
		domain.PaymentMethodCash,
		inputs,
	)
	if err != nil {
		t.Fatalf("cash must not use the QRIS ceiling: %v", err)
	}
	if len(items) != 1 || total != domain.MaxQRISTransactionTotal+1 {
		t.Fatalf("unexpected resolved cash transaction: items=%d total=%d", len(items), total)
	}
}
