package domain

import "testing"

func TestResolvePaymentAmount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mode   DataMode
		method PaymentMethod
		total  int64
		want   int64
	}{
		{name: "production qris", mode: DataModeProduction, method: PaymentMethodQRIS, total: 70_000, want: 70_000},
		{name: "sandbox qris", mode: DataModeSandbox, method: PaymentMethodQRIS, total: 70_000, want: 1_000},
		{name: "sandbox cash", mode: DataModeSandbox, method: PaymentMethodCash, total: 70_000, want: 70_000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ResolvePaymentAmount(test.mode, test.method, test.total); got != test.want {
				t.Fatalf("ResolvePaymentAmount()=%d, want %d", got, test.want)
			}
		})
	}
}

func TestPaymentPolicyIsSessionBoundAndLegacyByDefault(t *testing.T) {
	t.Parallel()
	for _, total := range []int64{500, 1_000, 25_000, 150_000} {
		for _, policy := range []SandboxQRISPolicy{SandboxQRISPolicyFixed1000, SandboxQRISPolicyTransactionTotal} {
			want := int64(1_000)
			if policy == SandboxQRISPolicyTransactionTotal {
				want = total
			}
			if got := ResolvePaymentAmount(DataModeSandbox, PaymentMethodQRIS, total, policy); got != want {
				t.Fatalf("policy %s total %d: amount %d, want %d", policy, total, got, want)
			}
			for _, mode := range []DataMode{DataModeProduction, DataModeSandbox} {
				if got := ResolvePaymentAmount(mode, PaymentMethodCash, total, policy); got != total {
					t.Fatalf("cash unexpectedly affected by policy: %s %s %d", mode, policy, got)
				}
			}
			if got := ResolvePaymentAmount(DataModeProduction, PaymentMethodQRIS, total, policy); got != total {
				t.Fatalf("Production QRIS unexpectedly affected by policy: %s %d", policy, got)
			}
		}
	}
	if got := (Principal{ProtocolVersion: 3}).EffectiveSandboxQRISPolicy(); got != SandboxQRISPolicyFixed1000 {
		t.Fatalf("protocol alone must not invent a missing session policy: %s", got)
	}
	for _, version := range []int{0, 1, 2, 3, 4} {
		negotiated := NegotiatedClientProtocolVersion(version)
		want := SandboxQRISPolicyFixed1000
		if version >= 3 {
			want = SandboxQRISPolicyTransactionTotal
		}
		if got := SandboxQRISPolicyForProtocol(negotiated); got != want {
			t.Fatalf("requested protocol %d negotiated policy %s, want %s", version, got, want)
		}
	}
}

func TestDataModeContract(t *testing.T) {
	t.Parallel()

	if !DataModeProduction.Valid() || !DataModeSandbox.Valid() || DataMode("live").Valid() {
		t.Fatal("data mode contract must be production|sandbox")
	}
	if EffectiveDataSpaceID([16]byte{}) != [16]byte{} {
		t.Fatal("missing data scope must not acquire production access")
	}
}

func TestSyncCursorIsGenerationBoundInSandbox(t *testing.T) {
	t.Parallel()

	if got := EncodeSyncCursor(DataModeProduction, 0, 41); got != "41" {
		t.Fatalf("production cursor=%q", got)
	}
	if got := EncodeSyncCursor(DataModeSandbox, 7, 41); got != "sandbox:7:41" {
		t.Fatalf("sandbox cursor=%q", got)
	}
	if got, err := DecodeSyncCursor(DataModeProduction, 0, "41"); err != nil || got != 41 {
		t.Fatalf("decoded production cursor=%d error=%v", got, err)
	}
	if got, err := DecodeSyncCursor(DataModeSandbox, 7, "sandbox:7:41"); err != nil || got != 41 {
		t.Fatalf("decoded sandbox cursor=%d error=%v", got, err)
	}
	if _, err := DecodeSyncCursor(DataModeSandbox, 8, "sandbox:7:41"); !IsCode(err, CodeSandboxGenerationRetired) {
		t.Fatalf("stale generation error=%v", err)
	}
	if _, err := DecodeSyncCursor(DataModeSandbox, 7, "41"); !IsCode(err, CodeSandboxGenerationRetired) {
		t.Fatalf("unbound sandbox cursor error=%v", err)
	}
}
