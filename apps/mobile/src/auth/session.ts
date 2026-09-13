import type { LoginResponse } from "@/api/contracts";
import {
  INITIAL_TENANT_ID,
  type Session,
  type ContextKind,
  type TenantSummary,
} from "@/domain/types";

export function sessionFromLoginResponse(result: LoginResponse): Session {
  const dataMode = result.dataMode === "sandbox" ? "sandbox" : "production";
  const principal = result.user as typeof result.user & {
    contextKind?: ContextKind;
    tenantId?: string | null;
    membershipId?: string | null;
    tenant?: TenantSummary | null;
    isPlatformAdmin?: boolean;
  };
  const contextKind = result.contextKind ?? principal.contextKind ?? "tenant";
  const tenantId = result.tenantId ?? principal.tenantId;
  if (
    contextKind === "tenant" &&
    result.contextKind &&
    (!tenantId || !result.dataSpaceId)
  )
    throw new Error(
      "Sesi bisnis tidak memiliki ruang data yang valid. Masuk kembali atau perbarui aplikasi.",
    );
  return {
    token: result.sessionToken,
    sessionId: result.sessionId,
    user: result.user,
    establishedAt: new Date().toISOString(),
    dataMode,
    dataSpaceId: contextKind === "tenant" ? result.dataSpaceId : null,
    sandboxGeneration:
      dataMode === "sandbox" && result.sandboxGeneration > 0
        ? result.sandboxGeneration
        : null,
    contextKind,
    tenantId: contextKind === "tenant" ? (tenantId ?? INITIAL_TENANT_ID) : null,
    membershipId: result.membershipId ?? principal.membershipId ?? null,
    tenant:
      result.tenant ??
      principal.tenant ??
      (contextKind === "tenant" && !tenantId
        ? {
            id: INITIAL_TENANT_ID,
            name: "Telomoyo",
            slug: "telomoyo",
            status: "active",
          }
        : null),
    isPlatformAdmin:
      result.isPlatformAdmin ?? principal.isPlatformAdmin ?? false,
    protocolVersion: result.protocolVersion ?? 2,
    sandboxQrisPolicy:
      result.sandboxQrisPolicy === "transaction_total"
        ? "transaction_total"
        : "fixed_1000",
  };
}
