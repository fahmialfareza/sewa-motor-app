package domain

import (
	"encoding/json"
	"testing"
)

func TestNormalizeLegacyTransactionSnapshotAtReadBoundary(t *testing.T) {
	t.Parallel()

	original := json.RawMessage(`{
		"occurredAt":"2026-01-01T00:00:00Z",
		"qrisPayloadHash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"items":[],"subtotal":0,"total":0
	}`)
	normalized := NormalizeTransactionSnapshot(original, 4)

	var value map[string]any
	if err := json.Unmarshal(normalized, &value); err != nil {
		t.Fatal(err)
	}
	if value["paymentMethod"] != string(PaymentMethodLegacy) ||
		value["paymentStatus"] != string(PaymentStatusSuccess) ||
		value["paymentConfirmedRevision"] != float64(4) {
		t.Fatalf("legacy snapshot was not normalized: %s", normalized)
	}
	if _, exists := value["qrisPayloadHash"]; exists {
		t.Fatalf("legacy snapshot retained a QRIS binding: %s", normalized)
	}
	if string(original) == string(normalized) {
		t.Fatal("normalization unexpectedly reused the legacy JSON")
	}
}

func TestNormalizeCurrentTransactionSnapshotPreservesPaymentState(t *testing.T) {
	t.Parallel()

	const hash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	original := json.RawMessage(`{
		"paymentMethod":"qris",
		"qrisPayloadHash":"` + hash + `",
		"paymentStatus":"success",
		"paymentConfirmedRevision":3
	}`)
	normalized := NormalizeTransactionSnapshot(original, 3)

	var value map[string]any
	if err := json.Unmarshal(normalized, &value); err != nil {
		t.Fatal(err)
	}
	if value["paymentMethod"] != string(PaymentMethodQRIS) ||
		value["qrisPayloadHash"] != hash ||
		value["paymentStatus"] != string(PaymentStatusSuccess) ||
		value["paymentConfirmedRevision"] != float64(3) {
		t.Fatalf("current snapshot payment state changed: %s", normalized)
	}
}

func TestNormalizeLegacyIdempotencyTransactionResult(t *testing.T) {
	t.Parallel()

	normalized := NormalizeLegacyTransactionResult(Transaction{Revision: 6})
	if normalized.PaymentMethod != PaymentMethodLegacy ||
		normalized.PaymentStatus != PaymentStatusSuccess ||
		normalized.PaymentConfirmedRevision == nil ||
		*normalized.PaymentConfirmedRevision != 6 {
		t.Fatalf("legacy idempotency result was not normalized: %+v", normalized)
	}
}
