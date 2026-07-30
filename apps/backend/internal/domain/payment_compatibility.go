package domain

import "encoding/json"

// NormalizeTransactionSnapshot upgrades immutable snapshot JSON at read time.
// Rows written before payment tracking are intentionally left untouched in the
// database and retain their original append-only signatures/audit history.
func NormalizeTransactionSnapshot(snapshot json.RawMessage, revision int) json.RawMessage {
	if len(snapshot) == 0 || revision < 1 {
		return snapshot
	}

	var value map[string]any
	if err := json.Unmarshal(snapshot, &value); err != nil {
		return snapshot
	}
	if value == nil {
		return snapshot
	}

	methodText, _ := value["paymentMethod"].(string)
	method := PaymentMethod(methodText)
	if !method.Valid() || method == PaymentMethodLegacy {
		value["paymentMethod"] = PaymentMethodLegacy
		value["paymentStatus"] = PaymentStatusSuccess
		value["paymentConfirmedRevision"] = revision
		delete(value, "qrisPayloadHash")
		return marshalNormalizedSnapshot(snapshot, value)
	}
	if method != PaymentMethodQRIS {
		delete(value, "qrisPayloadHash")
	}

	statusText, _ := value["paymentStatus"].(string)
	status := PaymentStatus(statusText)
	if !status.Valid() {
		status = PaymentStatusPending
		value["paymentStatus"] = status
	}
	if status == PaymentStatusSuccess {
		if confirmed, ok := jsonNumberAsPositiveInt(value["paymentConfirmedRevision"]); !ok {
			value["paymentConfirmedRevision"] = revision
		} else {
			value["paymentConfirmedRevision"] = confirmed
		}
	} else {
		value["paymentConfirmedRevision"] = nil
	}
	return marshalNormalizedSnapshot(snapshot, value)
}

// NormalizeLegacyTransactionResult upgrades successful append-only
// idempotency responses written before payment fields existed.
func NormalizeLegacyTransactionResult(transaction Transaction) Transaction {
	if !transaction.PaymentMethod.Valid() ||
		transaction.PaymentMethod == PaymentMethodLegacy {
		transaction.PaymentMethod = PaymentMethodLegacy
		transaction.PaymentStatus = PaymentStatusSuccess
		confirmedRevision := transaction.Revision
		transaction.PaymentConfirmedRevision = &confirmedRevision
		transaction.QrisPayloadHash = nil
		return transaction
	}
	if transaction.PaymentMethod != PaymentMethodQRIS {
		transaction.QrisPayloadHash = nil
	}
	if !transaction.PaymentStatus.Valid() {
		transaction.PaymentStatus = PaymentStatusPending
		transaction.PaymentConfirmedRevision = nil
	}
	return transaction
}

func jsonNumberAsPositiveInt(value any) (int, bool) {
	number, ok := value.(float64)
	if !ok || number < 1 || number != float64(int(number)) {
		return 0, false
	}
	return int(number), true
}

func marshalNormalizedSnapshot(
	fallback json.RawMessage,
	value map[string]any,
) json.RawMessage {
	normalized, err := json.Marshal(value)
	if err != nil {
		return fallback
	}
	return normalized
}
