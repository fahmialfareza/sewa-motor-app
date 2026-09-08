package domain

import (
	"fmt"
	"strconv"
	"strings"
)

const SandboxQRISPaymentAmount int64 = 1_000

// ResolvePaymentAmount returns the amount actually presented for payment. A
// sandbox QRIS payment intentionally charges a fixed Rp1,000 while transaction
// totals continue to model the real sale for analytics testing.
func ResolvePaymentAmount(mode DataMode, method PaymentMethod, total int64) int64 {
	if mode == DataModeSandbox && method == PaymentMethodQRIS {
		return SandboxQRISPaymentAmount
	}
	return total
}

// DecodeSyncCursor keeps production's historic decimal cursor compatible and
// binds sandbox cursors to the immutable generation carried by the session.
func DecodeSyncCursor(mode DataMode, generation int64, value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	if mode != DataModeSandbox {
		cursor, err := strconv.ParseInt(value, 10, 64)
		if err != nil || cursor < 0 {
			return 0, Validation("Cursor sinkronisasi tidak valid", nil)
		}
		return cursor, nil
	}
	prefix := fmt.Sprintf("sandbox:%d:", generation)
	if !strings.HasPrefix(value, prefix) {
		return 0, NewError(
			CodeSandboxGenerationRetired,
			"Generasi Sandbox telah diganti. Muat ulang data Sandbox untuk melanjutkan",
		)
	}
	cursor, err := strconv.ParseInt(strings.TrimPrefix(value, prefix), 10, 64)
	if err != nil || cursor < 0 {
		return 0, Validation("Cursor sinkronisasi tidak valid", nil)
	}
	return cursor, nil
}

func EncodeSyncCursor(mode DataMode, generation, cursor int64) string {
	if mode == DataModeSandbox {
		return fmt.Sprintf("sandbox:%d:%d", generation, cursor)
	}
	return strconv.FormatInt(cursor, 10)
}
