import type { ApiSchema } from "@sewa-motor/api-client";

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
export type LoginResponse = ApiSchema["LoginResult"];
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
