package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestTenantCursorScopeAndLegacyCompatibility(t *testing.T) {
	initial := Principal{ContextKind: ContextTenant, TenantID: InitialTenantID(), DataSpaceID: LiveDataSpaceID(), DataMode: DataModeProduction}
	other := Principal{ContextKind: ContextTenant, TenantID: uuid.New(), DataSpaceID: uuid.New(), DataMode: DataModeProduction}
	sandbox := other
	sandbox.DataMode, sandbox.DataSpaceID, sandbox.SandboxGeneration = DataModeSandbox, uuid.New(), 3
	for _, principal := range []Principal{initial, other, sandbox} {
		encoded := EncodeTenantSyncCursor(principal, 123)
		if cursor, err := DecodeTenantSyncCursor(principal, encoded); err != nil || cursor != 123 {
			t.Fatalf("round trip %q = %d, %v", encoded, cursor, err)
		}
		if cursor, err := DecodeTenantSyncCursor(principal, ""); err != nil || cursor != 0 {
			t.Fatalf("initial cursor: %d %v", cursor, err)
		}
	}
	if cursor, err := DecodeTenantSyncCursor(initial, "123"); err != nil || cursor != 123 {
		t.Fatalf("legacy initial production: %d %v", cursor, err)
	}
	if _, err := DecodeTenantSyncCursor(other, "123"); !IsCode(err, CodeValidation) {
		t.Fatalf("new tenant accepted legacy numeric cursor: %v", err)
	}
	if _, err := DecodeTenantSyncCursor(other, EncodeTenantSyncCursor(initial, 1)); !IsCode(err, CodeValidation) {
		t.Fatalf("cross tenant cursor: %v", err)
	}
	retired := sandbox
	retired.SandboxGeneration--
	retired.DataSpaceID = uuid.New()
	if _, err := DecodeTenantSyncCursor(sandbox, EncodeTenantSyncCursor(retired, 12)); !IsCode(err, CodeSandboxGenerationRetired) {
		t.Fatalf("retired cursor: %v", err)
	}
	legacySandbox := initial
	legacySandbox.DataMode, legacySandbox.DataSpaceID, legacySandbox.SandboxGeneration = DataModeSandbox, uuid.New(), 2
	if cursor, err := DecodeTenantSyncCursor(legacySandbox, "sandbox:2:123"); err != nil || cursor != 123 {
		t.Fatalf("legacy sandbox: %d %v", cursor, err)
	}
	for _, malformed := range []string{"tenant:", EncodeTenantSyncCursor(other, -1), EncodeTenantSyncCursor(other, 1) + ":0"} {
		if _, err := DecodeTenantSyncCursor(other, malformed); !IsCode(err, CodeValidation) {
			t.Fatalf("malformed cursor %q: %v", malformed, err)
		}
	}
	for _, principal := range []Principal{{}, {ContextKind: ContextAccount}, {ContextKind: ContextPlatform}, {ContextKind: ContextTenant, TenantID: InitialTenantID()}} {
		if _, err := DecodeTenantSyncCursor(principal, ""); !IsCode(err, CodeContextRequired) {
			t.Fatalf("missing business context accepted: %+v %v", principal, err)
		}
	}
}
