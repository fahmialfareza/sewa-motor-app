package domain

import (
	"fmt"
	"strconv"
	"strings"
)

const SandboxQRISPaymentAmount int64 = 1_000

type SandboxQRISPolicy string

const (
	SandboxQRISPolicyFixed1000        SandboxQRISPolicy = "fixed_1000"
	SandboxQRISPolicyTransactionTotal SandboxQRISPolicy = "transaction_total"
	CurrentClientProtocolVersion                        = 3
)

func (policy SandboxQRISPolicy) Valid() bool {
	return policy == SandboxQRISPolicyFixed1000 || policy == SandboxQRISPolicyTransactionTotal
}

func NegotiatedClientProtocolVersion(requested int) int {
	if requested >= CurrentClientProtocolVersion {
		return CurrentClientProtocolVersion
	}
	return 2
}

func SandboxQRISPolicyForProtocol(protocol int) SandboxQRISPolicy {
	if protocol >= CurrentClientProtocolVersion {
		return SandboxQRISPolicyTransactionTotal
	}
	return SandboxQRISPolicyFixed1000
}

func (principal Principal) EffectiveSandboxQRISPolicy() SandboxQRISPolicy {
	if principal.SandboxQRISPolicy.Valid() {
		return principal.SandboxQRISPolicy
	}
	// Missing fields represent a persisted legacy session, never a new capability.
	return SandboxQRISPolicyFixed1000
}

// ResolvePaymentAmount defaults to the legacy policy for old callers. Production
// and cash always use the total. Mutation handlers must pass the policy persisted
// on the signed origin session, not the session uploading an offline operation.
func ResolvePaymentAmount(mode DataMode, method PaymentMethod, total int64, policies ...SandboxQRISPolicy) int64 {
	policy := SandboxQRISPolicyFixed1000
	if len(policies) > 0 {
		policy = policies[0]
	}
	if mode == DataModeSandbox && method == PaymentMethodQRIS && policy != SandboxQRISPolicyTransactionTotal {
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
