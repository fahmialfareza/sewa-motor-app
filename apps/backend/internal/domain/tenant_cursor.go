package domain

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// Tenant sync cursors bind the stream to both its business and immutable space.
// The original installation may resume a cursor issued before the tenant rollout.
func DecodeTenantSyncCursor(principal Principal, value string) (int64, error) {
	if principal.ContextKind != ContextTenant || principal.TenantID == uuid.Nil || principal.DataSpaceID == uuid.Nil {
		return 0, NewError(CodeContextRequired, "Pilih usaha untuk sinkronisasi")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	if !strings.HasPrefix(value, "tenant:") {
		if principal.TenantID != InitialTenantID() {
			return 0, Validation("Cursor sinkronisasi berasal dari usaha lain", nil)
		}
		return DecodeSyncCursor(principal.EffectiveDataMode(), principal.SandboxGeneration, value)
	}
	parts := strings.Split(value, ":")
	if len(parts) != 5 || parts[1] != principal.TenantID.String() {
		return 0, Validation("Cursor sinkronisasi berasal dari usaha lain", nil)
	}
	if parts[2] != principal.DataSpaceID.String() || parts[3] != strconv.FormatInt(principal.SandboxGeneration, 10) {
		if principal.EffectiveDataMode() == DataModeSandbox {
			return 0, NewError(CodeSandboxGenerationRetired, "Generasi Sandbox telah diganti. Muat ulang data Sandbox untuk melanjutkan")
		}
		return 0, Validation("Cursor sinkronisasi berasal dari ruang data lain", nil)
	}
	cursor, err := strconv.ParseInt(parts[4], 10, 64)
	if err != nil || cursor < 0 {
		return 0, Validation("Cursor sinkronisasi tidak valid", nil)
	}
	return cursor, nil
}

func EncodeTenantSyncCursor(principal Principal, cursor int64) string {
	return fmt.Sprintf("tenant:%s:%s:%d:%d", principal.TenantID, principal.DataSpaceID, principal.SandboxGeneration, cursor)
}
