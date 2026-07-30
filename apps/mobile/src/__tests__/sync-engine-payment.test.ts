import type { Session, Transaction } from "@/domain/types";
import { runSync } from "@/sync/engine";

const QRIS_PAYLOAD_HASH =
  "9185bbfe94bb008d611da515fc94c2f3ad5f0c3fbfe278d8bdb463f9ce1cf500";

const mockNetworkFetch = jest.fn();
const mockApiRequest = jest.fn<Promise<unknown>, unknown[]>();
const mockGetOutboxOperations = jest.fn();
const mockGetSyncMetadata = jest.fn();
const mockGetTransaction = jest.fn();
const mockMarkOutboxResult = jest.fn();
const mockApplyRemoteChanges = jest.fn();
const mockSetSyncError = jest.fn();

jest.mock("@react-native-community/netinfo", () => ({
  __esModule: true,
  default: {
    fetch: (...args: unknown[]) => mockNetworkFetch(...args),
  },
}));

jest.mock("@/api/client", () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}));

jest.mock("@/db/repositories", () => ({
  applyRemoteChanges: (...args: unknown[]) => mockApplyRemoteChanges(...args),
  getOutboxOperations: (...args: unknown[]) => mockGetOutboxOperations(...args),
  getSyncMetadata: (...args: unknown[]) => mockGetSyncMetadata(...args),
  getTransaction: (...args: unknown[]) => mockGetTransaction(...args),
  markOutboxResult: (...args: unknown[]) => mockMarkOutboxResult(...args),
  setSyncError: (...args: unknown[]) => mockSetSyncError(...args),
}));

jest.mock("@/security/secure-store", () => ({
  readTerminalIdentity: jest.fn(),
  writeSession: jest.fn(),
}));

jest.mock("@/security/terminal-identity", () => ({
  markTerminalEnrolled: jest.fn(),
  markTerminalRevoked: jest.fn(),
}));

const session: Session = {
  token: "session-token",
  sessionId: "session-1",
  establishedAt: "2026-07-29T00:00:00.000Z",
  user: {
    id: "actor-1",
    fullName: "Andi",
    username: "andi",
    role: "admin",
    active: true,
    mustChangePassword: false,
  },
};

const operation = {
  operationId: "operation-1",
  aggregateId: "transaction-1",
  operation: {
    operationId: "operation-1",
    aggregate: "transaction",
    aggregateId: "transaction-1",
    action: "set_payment_status",
  },
  signature: "signature",
  attempts: 0,
};

const currentTransaction: Transaction = {
  id: operation.aggregateId,
  revision: 1,
  occurredAt: "2026-07-29T00:00:00.000Z",
  subtotal: 70_000,
  total: 70_000,
  originActorId: session.user.id,
  originActorName: session.user.fullName,
  updatedActorName: session.user.fullName,
  terminalId: "terminal-1",
  syncState: "pending",
  printState: "pending",
  paymentMethod: "cash",
  paymentStatus: "pending",
  paymentConfirmedRevision: null,
  qrisPayloadHash: null,
  deletedAt: null,
  items: [
    {
      id: "local-item-1",
      packageId: "00000000-0000-4000-8000-000000000001",
      packageRevision: 1,
      name: "Paket Standar",
      description: "Paket",
      accent: "standard",
      unitPrice: 70_000,
      quantity: 1,
      lineTotal: 70_000,
    },
  ],
};

const serverSnapshot = {
  occurredAt: "2026-07-29T07:02:00+07:00",
  paymentMethod: "qris" as const,
  paymentStatus: "success" as const,
  paymentConfirmedRevision: 2,
  qrisPayloadHash: QRIS_PAYLOAD_HASH,
  items: [
    {
      packageId: "00000000-0000-4000-8000-000000000001",
      packageRevision: 1,
      name: "Paket Standar",
      description: "Paket dari server",
      unitPrice: 70_000,
      quantity: 2,
      lineTotal: 140_000,
    },
  ],
  subtotal: 140_000,
  total: 140_000,
};

function arrangeSyncPush(result: Record<string, unknown>): void {
  mockGetOutboxOperations
    .mockResolvedValueOnce([operation])
    .mockResolvedValueOnce([]);
  mockApiRequest
    .mockResolvedValueOnce({ results: [result] })
    .mockResolvedValueOnce({
      changes: [],
      cursor: "cursor-1",
      hasMore: false,
    });
}

describe("payment conflict sync handling", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockNetworkFetch.mockResolvedValue({
      isConnected: true,
      isInternetReachable: true,
    });
    mockGetSyncMetadata.mockResolvedValue({ cursor: null });
    mockGetTransaction.mockResolvedValue(null);
    mockMarkOutboxResult.mockResolvedValue(undefined);
    mockApplyRemoteChanges.mockResolvedValue(undefined);
    mockSetSyncError.mockResolvedValue(undefined);
  });

  it("reconciles a valid payment conflict as a terminal authoritative result", async () => {
    mockGetTransaction.mockResolvedValueOnce(currentTransaction);
    arrangeSyncPush({
      operationId: operation.operationId,
      aggregateId: operation.aggregateId,
      status: "conflict",
      conflict: {
        kind: "payment_state",
        reason: "revision_changed",
        baseRevision: 1,
        currentRevision: 2,
        requestedStatus: "success",
        paymentStatus: "success",
        paymentConfirmedRevision: 2,
        serverSnapshot,
      },
      error: null,
    });

    await expect(runSync(session)).resolves.toMatchObject({ conflicts: 1 });
    expect(mockMarkOutboxResult).toHaveBeenCalledWith(
      operation.operationId,
      operation.aggregateId,
      {
        kind: "payment-conflict",
        message: "Status pembayaran berubah di server. Muat ulang transaksi.",
        paymentStatus: "success",
        paymentConfirmedRevision: 2,
        authoritative: {
          ...currentTransaction,
          revision: 2,
          occurredAt: "2026-07-29T00:02:00.000Z",
          subtotal: serverSnapshot.subtotal,
          total: serverSnapshot.total,
          syncState: "error",
          paymentMethod: "qris",
          paymentStatus: "success",
          paymentConfirmedRevision: 2,
          qrisPayloadHash: QRIS_PAYLOAD_HASH,
          items: [
            {
              id: `${operation.aggregateId}:2:1`,
              ...serverSnapshot.items[0],
              accent: "standard",
            },
          ],
        },
      },
    );
  });

  it("terminates malformed payment conflict data without applying it locally", async () => {
    arrangeSyncPush({
      operationId: operation.operationId,
      aggregateId: operation.aggregateId,
      status: "conflict",
      conflict: {
        kind: "payment_state",
        currentRevision: 2,
        paymentStatus: "success",
        paymentConfirmedRevision: null,
      },
      error: null,
    });

    await expect(runSync(session)).resolves.toMatchObject({ conflicts: 1 });
    expect(mockMarkOutboxResult).toHaveBeenCalledWith(
      operation.operationId,
      operation.aggregateId,
      {
        kind: "rejected",
        message: "Server mengembalikan konflik yang tidak dapat diproses.",
      },
    );
  });
});
