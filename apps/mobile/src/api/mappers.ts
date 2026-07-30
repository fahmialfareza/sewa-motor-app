import type {
  ApiPackage,
  ApiTransaction,
  ApiTransactionSnapshot,
} from "@/api/contracts";
import type {
  RentalPackage,
  Transaction,
  TransactionItem,
} from "@/domain/types";
import { normalizeQrisPayloadHash } from "@/domain/qris";
import { normalizeUtcTimestamp } from "@/utils/time";

const STANDARD_ID = "00000000-0000-4000-8000-000000000001";
const SUNRISE_ID = "00000000-0000-4000-8000-000000000002";

export function accentForPackage(id: string): RentalPackage["accent"] {
  if (id === STANDARD_ID) return "standard";
  if (id === SUNRISE_ID) return "sunrise";
  return "primary";
}

export function mapApiPackage(value: ApiPackage): RentalPackage {
  return {
    id: value.id,
    revision: value.currentRevision.revision,
    name: value.currentRevision.name,
    description: value.currentRevision.description,
    unitPrice: value.currentRevision.unitPrice,
    accent: accentForPackage(value.id),
    active: value.active,
    deletedAt: value.deletedAt,
  };
}

export function mapApiTransaction(value: ApiTransaction): Transaction {
  return {
    id: value.id,
    revision: value.revision,
    occurredAt: normalizeUtcTimestamp(value.occurredAt),
    subtotal: value.subtotal,
    total: value.total,
    originActorId: value.originActor.id,
    originActorName: value.originActor.fullName,
    updatedActorName: value.updatedBy.fullName,
    terminalId: value.terminal.id,
    syncState: "synced",
    printState: value.print.state,
    paymentMethod: value.paymentMethod,
    paymentStatus: value.paymentStatus,
    paymentConfirmedRevision: value.paymentConfirmedRevision,
    qrisPayloadHash:
      value.paymentMethod === "qris"
        ? normalizeQrisPayloadHash(value.qrisPayloadHash)
        : null,
    deletedAt: value.deletion?.deletedAt ?? null,
    items: value.items.map((item) => ({
      ...item,
      accent: accentForPackage(item.packageId),
    })),
  };
}

export function mergeSnapshot(
  base: Transaction,
  snapshot: ApiTransactionSnapshot,
  revision: number,
): Transaction {
  return {
    ...base,
    revision,
    occurredAt: normalizeUtcTimestamp(snapshot.occurredAt),
    subtotal: snapshot.subtotal,
    total: snapshot.total,
    paymentMethod: snapshot.paymentMethod,
    paymentStatus: snapshot.paymentStatus,
    paymentConfirmedRevision: snapshot.paymentConfirmedRevision,
    qrisPayloadHash:
      snapshot.paymentMethod === "qris"
        ? normalizeQrisPayloadHash(snapshot.qrisPayloadHash)
        : null,
    syncState: "conflict",
    items: snapshot.items.map((item, index): TransactionItem => ({
      id: `${base.id}:${revision}:${index + 1}`,
      ...item,
      accent: accentForPackage(item.packageId),
    })),
  };
}
