import NetInfo from "@react-native-community/netinfo";

import { ApiError, apiRequest, noticeAccessFailure } from "@/api/client";
import type {
  ApiPackage,
  ApiTransaction,
  PaymentStateConflictDetails,
  RevisionConflictDetails,
  SyncPullResponse,
  SyncPushResponse,
} from "@/api/contracts";
import { mapApiPackage, mapApiTransaction, mergeSnapshot } from "@/api/mappers";
import {
  applyRemoteChanges,
  getOutboxOperations,
  getSyncMetadata,
  getTransaction,
  markOutboxResult,
  setSyncError,
  type RemoteChange,
} from "@/db/repositories";
import type { Session } from "@/domain/types";
import { readTerminalIdentity, writeSession } from "@/security/secure-store";
import {
  markTerminalEnrolled,
  markTerminalRevoked,
} from "@/security/terminal-identity";
import { toUserFacingErrorMessage } from "@/utils/errors";
import { refreshTenantConfiguration } from "@/tenant/configuration";
import {
  blockedScopeReason,
  quarantineScope,
  SCOPE_ACCESS_CODES,
} from "@/tenant/quarantine";

let activeSync: { sessionId: string; promise: Promise<SyncSummary> } | null =
  null;

export interface SyncSummary {
  pushed: number;
  pulled: number;
  conflicts: number;
  completedAt: string;
}

export function runSync(session: Session): Promise<SyncSummary> {
  if (activeSync) {
    if (activeSync.sessionId === session.sessionId) return activeSync.promise;

    return activeSync.promise
      .catch(() => undefined)
      .then(() => runSync(session));
  }

  const promise = runSyncInternal(session).finally(() => {
    if (activeSync?.promise === promise) activeSync = null;
  });
  activeSync = { sessionId: session.sessionId, promise };
  return promise;
}

async function runSyncInternal(session: Session): Promise<SyncSummary> {
  if (session.contextKind && session.contextKind !== "tenant")
    return {
      pushed: 0,
      pulled: 0,
      conflicts: 0,
      completedAt: new Date().toISOString(),
    };
  if (session.token.startsWith("dev-only-")) {
    return {
      pushed: 0,
      pulled: 0,
      conflicts: 0,
      completedAt: new Date().toISOString(),
    };
  }

  const network = await NetInfo.fetch();
  if (!network.isConnected || network.isInternetReachable === false) {
    throw new Error("Tidak ada koneksi internet.");
  }

  let pushed = 0;
  let pulled = 0;
  let conflicts = 0;

  try {
    if (await blockedScopeReason(session))
      throw new Error(
        "Antrean bisnis dikarantina. Pilih ulang bisnis setelah pengelola memulihkan akses.",
      );
    while (true) {
      const batch = await getOutboxOperations(25, session);
      if (batch.length === 0) break;
      const response = await apiRequest<SyncPushResponse>("/sync/push", {
        method: "POST",
        token: session.token,
        body: {
          mutations: batch.map((item) => ({
            ...item.operation,
            signature: item.signature,
          })),
        },
      });

      const retiredGeneration = response.results.find(
        (result) => result.error?.code === "SANDBOX_GENERATION_RETIRED",
      );
      const accessFailure = response.results.find(
        (result) => result.error && SCOPE_ACCESS_CODES.has(result.error.code),
      );
      if (accessFailure?.error) {
        await noticeAccessFailure(session.token, accessFailure.error.code);
        throw new ApiError({
          status: 403,
          code: accessFailure.error.code,
          message: accessFailure.error.message,
        });
      }
      if (retiredGeneration?.error) {
        throw new ApiError({
          status: 409,
          code: retiredGeneration.error.code,
          message: retiredGeneration.error.message,
          details: retiredGeneration.error.details,
          requestId: retiredGeneration.error.requestId,
        });
      }

      for (const item of batch) {
        const result = response.results.find(
          (candidate) => candidate.operationId === item.operationId,
        );
        if (!result) {
          await markOutboxResult(
            item.operationId,
            item.aggregateId,
            {
              kind: "error",
              message: "Server tidak mengembalikan hasil operasi.",
            },
            session,
          );
          continue;
        }
        if (result.status === "applied" || result.status === "duplicate") {
          await markOutboxResult(
            item.operationId,
            item.aggregateId,
            { kind: "success" },
            session,
          );
          pushed += 1;
        } else if (
          result.status === "conflict" &&
          isRevisionConflict(result.conflict)
        ) {
          const current = await getTransaction(item.aggregateId, session);
          const local = current
            ? mergeSnapshot(
                current,
                result.conflict.localSnapshot,
                result.conflict.baseRevision + 1,
              )
            : null;
          if (local) {
            await markOutboxResult(
              item.operationId,
              item.aggregateId,
              {
                kind: "conflict",
                local,
                server: mergeSnapshot(
                  local,
                  result.conflict.serverSnapshot,
                  result.conflict.currentRevision,
                ),
              },
              session,
            );
            conflicts += 1;
          } else {
            await markOutboxResult(
              item.operationId,
              item.aggregateId,
              {
                kind: "rejected",
                message:
                  result.error?.message ??
                  "Konflik revisi tidak dapat dipetakan ke transaksi lokal.",
              },
              session,
            );
          }
        } else if (
          result.status === "conflict" &&
          isPaymentStateConflict(result.conflict)
        ) {
          const current = await getTransaction(item.aggregateId, session);
          if (!current) {
            await markOutboxResult(
              item.operationId,
              item.aggregateId,
              {
                kind: "rejected",
                message:
                  "Konflik pembayaran tidak dapat dipetakan ke transaksi lokal.",
              },
              session,
            );
            conflicts += 1;
            continue;
          }
          const authoritative = mergeSnapshot(
            current,
            result.conflict.serverSnapshot,
            result.conflict.currentRevision,
          );
          await markOutboxResult(
            item.operationId,
            item.aggregateId,
            {
              kind: "payment-conflict",
              message:
                result.error?.message ??
                "Status pembayaran berubah di server. Muat ulang transaksi.",
              paymentStatus: result.conflict.paymentStatus,
              paymentConfirmedRevision:
                result.conflict.paymentConfirmedRevision,
              authoritative: {
                ...authoritative,
                syncState: "error",
                paymentStatus: result.conflict.paymentStatus,
                paymentConfirmedRevision:
                  result.conflict.paymentConfirmedRevision,
              },
            },
            session,
          );
          conflicts += 1;
        } else if (result.status === "conflict") {
          await markOutboxResult(
            item.operationId,
            item.aggregateId,
            {
              kind: "rejected",
              message:
                result.error?.message ??
                "Server mengembalikan konflik yang tidak dapat diproses.",
            },
            session,
          );
          conflicts += 1;
        } else if (result.status === "rejected") {
          await markOutboxResult(
            item.operationId,
            item.aggregateId,
            {
              kind: "rejected",
              message: result.error?.message ?? "Operasi ditolak server.",
            },
            session,
          );
        } else {
          await markOutboxResult(
            item.operationId,
            item.aggregateId,
            {
              kind: "error",
              message: result.error?.message ?? "Operasi ditolak server.",
            },
            session,
          );
        }
      }
    }

    let cursor = (await getSyncMetadata(session)).cursor;
    const seenCursors = new Set(cursor ? [cursor] : []);
    while (true) {
      const query = new URLSearchParams({
        limit: "100",
        ...(cursor ? { cursor } : {}),
      });
      const response = await apiRequest<SyncPullResponse>(
        `/sync/pull?${query.toString()}`,
        { token: session.token },
      );
      if (response.hasMore && seenCursors.has(response.cursor)) {
        throw new ApiError({
          status: 200,
          code: "INVALID_SYNC_CURSOR",
          message:
            "Sinkronisasi dihentikan karena server tidak memajukan cursor data.",
        });
      }
      const localChanges: RemoteChange[] = [];
      for (const change of response.changes) {
        if (change.aggregate === "package") {
          localChanges.push({
            cursor: change.cursor,
            aggregate: "package",
            aggregateId: change.aggregateId,
            action: change.action,
            payload:
              change.payload && change.action === "upsert"
                ? mapApiPackage(change.payload as ApiPackage)
                : null,
            changedAt: change.changedAt,
          });
        } else if (change.aggregate === "transaction") {
          localChanges.push({
            cursor: change.cursor,
            aggregate: "transaction",
            aggregateId: change.aggregateId,
            action: change.action,
            payload:
              change.payload && change.action === "upsert"
                ? mapApiTransaction(change.payload as ApiTransaction)
                : null,
            changedAt: change.changedAt,
          });
        } else if (
          change.aggregate !== "tenant_profile" &&
          change.aggregate !== "tenant_qris"
        ) {
          localChanges.push({
            cursor: change.cursor,
            aggregate: change.aggregate,
            aggregateId: change.aggregateId,
            action: change.action,
            payload: change.payload,
            changedAt: change.changedAt,
          });
        }

        if (
          change.aggregate === "user" &&
          change.aggregateId === session.user.id
        ) {
          if (change.action === "delete" || !change.payload) {
            throw new Error("Akun perangkat ini dinonaktifkan di server.");
          }
          const currentUser = change.payload as Session["user"];
          if (!currentUser.active) {
            throw new Error("Akun perangkat ini dinonaktifkan di server.");
          }
          await writeSession({
            ...session,
            user: currentUser,
          });
        }
        if (change.aggregate === "terminal") {
          const identity = await readTerminalIdentity(
            session.tenantId ?? undefined,
          );
          if (identity?.serverTerminalId === change.aggregateId) {
            const terminal = change.payload as {
              active: boolean;
              revokedAt: string | null;
            } | null;
            if (
              change.action === "delete" ||
              !terminal ||
              terminal.active === false ||
              terminal.revokedAt !== null
            ) {
              await markTerminalRevoked(
                change.aggregateId,
                session.tenantId ?? undefined,
              );
              await noticeAccessFailure(session.token, "TERMINAL_REVOKED");
            } else {
              await markTerminalEnrolled(
                change.aggregateId,
                session.tenantId ?? undefined,
              );
            }
          }
        }
      }
      await applyRemoteChanges(localChanges, response.cursor, session);
      pulled += response.changes.length;
      cursor = response.cursor;
      if (!response.hasMore) break;
      seenCursors.add(cursor);
    }

    if (session.tenantId) await refreshTenantConfiguration(session);
    return {
      pushed,
      pulled,
      conflicts,
      completedAt: new Date().toISOString(),
    };
  } catch (error) {
    if (error instanceof ApiError && SCOPE_ACCESS_CODES.has(error.code))
      await quarantineScope(session, error.code);
    const message = toUserFacingErrorMessage(
      error,
      "Sinkronisasi belum berhasil. Coba lagi.",
    );
    await setSyncError(message, session);
    throw error;
  }
}

function isRevisionConflict(value: unknown): value is RevisionConflictDetails {
  if (!value || typeof value !== "object") return false;
  const conflict = value as Partial<RevisionConflictDetails>;
  return (
    Number.isInteger(conflict.baseRevision) &&
    (conflict.baseRevision ?? -1) >= 0 &&
    Number.isInteger(conflict.currentRevision) &&
    (conflict.currentRevision ?? 0) >= 1 &&
    typeof conflict.localSnapshot === "object" &&
    conflict.localSnapshot !== null &&
    typeof conflict.serverSnapshot === "object" &&
    conflict.serverSnapshot !== null
  );
}

function isPaymentStateConflict(
  value: unknown,
): value is PaymentStateConflictDetails {
  if (!value || typeof value !== "object") return false;
  const conflict = value as Record<string, unknown>;
  if (
    conflict.kind !== "payment_state" ||
    !Number.isInteger(conflict.currentRevision) ||
    Number(conflict.currentRevision) < 1 ||
    typeof conflict.serverSnapshot !== "object" ||
    conflict.serverSnapshot === null
  ) {
    return false;
  }
  if (conflict.paymentStatus === "success") {
    return (
      Number.isInteger(conflict.paymentConfirmedRevision) &&
      Number(conflict.paymentConfirmedRevision) >= 1
    );
  }
  return (
    (conflict.paymentStatus === "pending" ||
      conflict.paymentStatus === "failed") &&
    conflict.paymentConfirmedRevision === null
  );
}
