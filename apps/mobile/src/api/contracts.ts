import type { ApiSchema } from "@sewa-motor/api-client";
import type {
  BusinessProfile,
  ContextKind,
  Role,
  SandboxQrisPolicy,
  TenantSummary,
} from "@/domain/types";

/**
 * Mobile-facing aliases of the OpenAPI-generated schema. Keeping the names
 * close to the UI domain makes call sites readable without duplicating the
 * wire contract.
 */
export interface ApiEnvelope<T> {
  data: T;
  meta: ApiSchema["Meta"];
}

export type ApiErrorEnvelope = ApiSchema["ErrorEnvelope"];
export type ApiTerminal = ApiSchema["Terminal"];
export type LoginResponse = ApiSchema["LoginResult"] & {
  contextKind?: ContextKind;
  tenantId?: string | null;
  membershipId?: string | null;
  tenant?: TenantSummary | null;
  isPlatformAdmin?: boolean;
  protocolVersion?: number;
  sandboxQrisPolicy?: SandboxQrisPolicy;
};
export interface AuthContextsResponse {
  tenants: { tenant: TenantSummary; membershipId: string; role: Role }[];
  platformAdmin?: boolean;
  canManageOrganization?: boolean;
  tenantProvisioningEnabled?: boolean;
}
export interface TenantQrisResponse {
  revision: number;
  activePayloadHash: string | null;
  payloads: { payloadHash: string; staticPayload: string; revision: number }[];
}
export interface TenantInvitation {
  id: string;
  role: Role;
  code?: string;
  expiresAt: string;
  revokedAt?: string | null;
  acceptedAt?: string | null;
}
export type TenantProfileResponse = BusinessProfile;
export type ApiPackage = ApiSchema["Package"];
export type ApiTransactionItem = ApiSchema["TransactionItem"];
export type ApiTransaction = ApiSchema["Transaction"];
export type ApiTransactionSnapshot = ApiSchema["TransactionSnapshot"];
export type RevisionConflictDetails = ApiSchema["RevisionConflictDetails"];
export type PaymentStateConflictDetails =
  ApiSchema["PaymentStateConflictDetails"];
export type SyncPushResult = ApiSchema["SyncMutationResult"];
export type SyncPushResponse = ApiSchema["SyncPushResult"];
export type ApiSyncChange = ApiSchema["SyncChange"];
export type SyncPullResponse = ApiSchema["SyncPullResult"];
export type UserListResponse = ApiSchema["User"][];
export type PackageListResponse = ApiSchema["Package"][];

export type SandboxStatusResponse = ApiSchema["SandboxStatus"];
export type SandboxResetResponse = ApiSchema["SandboxResetResult"];
