import NetInfo from "@react-native-community/netinfo";

import { apiRequest } from "@/api/client";
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
    while (true) {
      const batch = await getOutboxOperations(25);
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

      for (const item of batch) {
        const result = response.results.find(
          (candidate) => candidate.operationId === item.operationId,
        );
        if (!result) {
          await markOutboxResult(item.operationId, item.aggregateId, {
            kind: "error",
            message: "Server tidak mengembalikan hasil operasi.",
          });
          continue;
        }
        if (result.status === "applied" || result.status === "duplicate") {
          await markOutboxResult(item.operationId, item.aggregateId, {
            kind: "success",
          });
          pushed += 1;
        } else if (
          result.status === "conflict" &&
          isRevisionConflict(result.conflict)
        ) {
          const current = await getTransaction(item.aggregateId);
          const local = current
            ? mergeSnapshot(
                current,
                result.conflict.localSnapshot,
                result.conflict.baseRevision + 1,
              )
            : null;
          if (local) {
            await markOutboxResult(item.operationId, item.aggregateId, {
              kind: "conflict",
              local,
              server: mergeSnapshot(
                local,
                result.conflict.serverSnapshot,
                result.conflict.currentRevision,
              ),
            });
            conflicts += 1;
          } else {
            await markOutboxResult(item.operationId, item.aggregateId, {
              kind: "rejected",
              message:
                result.error?.message ??
                "Konflik revisi tidak dapat dipetakan ke transaksi lokal.",
            });
          }
        } else if (
          result.status === "conflict" &&
          isPaymentStateConflict(result.conflict)
        ) {
          const current = await getTransaction(item.aggregateId);
          if (!current) {
            await markOutboxResult(item.operationId, item.aggregateId, {
              kind: "rejected",
              message:
                "Konflik pembayaran tidak dapat dipetakan ke transaksi lokal.",
            });
            conflicts += 1;
            continue;
          }
          const authoritative = mergeSnapshot(
            current,
            result.conflict.serverSnapshot,
            result.conflict.currentRevision,
          );
          await markOutboxResult(item.operationId, item.aggregateId, {
            kind: "payment-conflict",
            message:
              result.error?.message ??
              "Status pembayaran berubah di server. Muat ulang transaksi.",
            paymentStatus: result.conflict.paymentStatus,
            paymentConfirmedRevision: result.conflict.paymentConfirmedRevision,
            authoritative: {
              ...authoritative,
              syncState: "error",
              paymentStatus: result.conflict.paymentStatus,
              paymentConfirmedRevision:
                result.conflict.paymentConfirmedRevision,
            },
          });
          conflicts += 1;
        } else if (result.status === "conflict") {
          await markOutboxResult(item.operationId, item.aggregateId, {
            kind: "rejected",
            message:
              result.error?.message ??
              "Server mengembalikan konflik yang tidak dapat diproses.",
          });
          conflicts += 1;
        } else if (result.status === "rejected") {
          await markOutboxResult(item.operationId, item.aggregateId, {
            kind: "rejected",
            message: result.error?.message ?? "Operasi ditolak server.",
          });
        } else {
          await markOutboxResult(item.operationId, item.aggregateId, {
            kind: "error",
            message: result.error?.message ?? "Operasi ditolak server.",
          });
        }
      }
    }

    let cursor = (await getSyncMetadata()).cursor;
    for (let page = 0; page < 20; page += 1) {
      const query = new URLSearchParams({
        limit: "100",
        ...(cursor ? { cursor } : {}),
      });
      const response = await apiRequest<SyncPullResponse>(
        `/sync/pull?${query.toString()}`,
        { token: session.token },
      );
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
        } else {
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
          const identity = await readTerminalIdentity();
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
              await markTerminalRevoked(change.aggregateId);
            } else {
              await markTerminalEnrolled(change.aggregateId);
            }
          }
        }
      }
      await applyRemoteChanges(localChanges, response.cursor);
      pulled += response.changes.length;
      cursor = response.cursor;
      if (!response.hasMore) break;
    }

    return {
      pushed,
      pulled,
      conflicts,
      completedAt: new Date().toISOString(),
    };
  } catch (error) {
    const message = toUserFacingErrorMessage(
      error,
      "Sinkronisasi belum berhasil. Coba lagi.",
    );
    await setSyncError(message);
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
