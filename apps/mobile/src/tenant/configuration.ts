import { apiRequest } from "@/api/client";
import type { TenantQrisResponse } from "@/api/contracts";
import { getDatabase } from "@/db/client";
import {
  INITIAL_TENANT_ID,
  type BusinessProfile,
  type LocalScope,
  type Session,
} from "@/domain/types";
import { clearQrisConfig, writeQrisConfig } from "@/security/secure-store";

export const LEGACY_BUSINESS_PROFILE: BusinessProfile = {
  businessName: "Telomoyo",
  address: null,
  phone: null,
  revision: 1,
};

export async function readTenantConfiguration<T>(
  kind: "profile" | "qris",
  scope?: LocalScope,
): Promise<T | null> {
  const { sqlite } = await getDatabase(scope);
  const row = await sqlite.getFirstAsync<{ payload_json: string }>(
    "SELECT payload_json FROM tenant_configuration WHERE kind = ?",
    kind,
  );
  return row ? (JSON.parse(row.payload_json) as T) : null;
}

export async function cacheTenantConfiguration(
  kind: "profile" | "qris",
  payload: unknown,
  scope: LocalScope,
): Promise<void> {
  const { sqlite } = await getDatabase(scope);
  await sqlite.runAsync(
    "INSERT INTO tenant_configuration(kind, payload_json) VALUES (?, ?) ON CONFLICT(kind) DO UPDATE SET payload_json = excluded.payload_json",
    kind,
    JSON.stringify(payload),
  );
  if (kind === "qris") {
    const config = payload as TenantQrisResponse;
    const active = config.payloads.find(
      (item) => item.payloadHash === config.activePayloadHash,
    );
    const tenantId =
      typeof scope === "string" ? undefined : (scope.tenantId ?? undefined);
    if (active)
      await writeQrisConfig({ staticPayload: active.staticPayload }, tenantId);
    else await clearQrisConfig(tenantId);
  }
}

export async function refreshTenantConfiguration(
  session: Session,
): Promise<void> {
  const [profile, qris] = await Promise.all([
    apiRequest<BusinessProfile>("/tenant/profile", { token: session.token }),
    apiRequest<TenantQrisResponse>("/tenant/qris", { token: session.token }),
  ]);
  await cacheTenantConfiguration("profile", profile, session);
  await cacheTenantConfiguration("qris", qris, session);
}

export async function receiptProfileForSession(
  session: Session,
): Promise<BusinessProfile> {
  const profile = await readTenantConfiguration<BusinessProfile>(
    "profile",
    session,
  );
  if (profile) return profile;
  if (session.tenantId === INITIAL_TENANT_ID) return LEGACY_BUSINESS_PROFILE;
  throw new Error(
    "Profil bisnis belum tersinkron. Hubungkan internet sebelum membuat transaksi pertama.",
  );
}

export async function qrisPayloadForTransaction(
  hash: string,
  scope?: LocalScope,
): Promise<string | null> {
  const config = await readTenantConfiguration<TenantQrisResponse>(
    "qris",
    scope,
  );
  return (
    config?.payloads.find((item) => item.payloadHash === hash)?.staticPayload ??
    null
  );
}
