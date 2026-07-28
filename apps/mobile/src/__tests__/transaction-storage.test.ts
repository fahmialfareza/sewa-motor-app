import type { SQLiteDatabase } from "expo-sqlite";

import { applyRemoteChanges } from "@/db/repositories";
import { latestMigrationVersion, runMigrations } from "@/db/migrations";
import type { Transaction } from "@/domain/types";

const mockRunAsync = jest.fn<Promise<unknown>, unknown[]>();
const mockWithTransactionAsync = jest.fn(
  async (callback: () => Promise<void>) => callback(),
);

jest.mock("@/db/client", () => ({
  getDatabase: async () => ({
    sqlite: {
      runAsync: (...args: unknown[]) => mockRunAsync(...args),
      withTransactionAsync: (callback: () => Promise<void>) =>
        mockWithTransactionAsync(callback),
    },
  }),
}));

const remoteTransaction: Transaction = {
  id: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
  revision: 1,
  occurredAt: "2026-07-28T00:00:00.000Z",
  subtotal: 70_000,
  total: 70_000,
  originActorId: "owner",
  originActorName: "Owner",
  updatedActorName: "Owner",
  terminalId: "terminal",
  syncState: "synced",
  printState: "pending",
  deletedAt: null,
  items: [
    {
      id: "server-item-id",
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

describe("transaction item storage", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockRunAsync.mockResolvedValue({});
  });

  it("replaces the item set for a synced revision before inserting it", async () => {
    await applyRemoteChanges(
      [
        {
          cursor: "1",
          aggregate: "transaction",
          action: "upsert",
          aggregateId: remoteTransaction.id,
          payload: remoteTransaction,
          changedAt: "2026-07-28T00:00:01.000Z",
        },
      ],
      "1",
    );

    const calls = mockRunAsync.mock.calls;
    const deleteIndex = calls.findIndex(([sql]) =>
      String(sql).includes("DELETE FROM transaction_items"),
    );
    const insertIndex = calls.findIndex(([sql]) =>
      String(sql).includes("INSERT OR IGNORE INTO transaction_items"),
    );

    expect(deleteIndex).toBeGreaterThanOrEqual(0);
    expect(insertIndex).toBeGreaterThan(deleteIndex);
    expect(calls[deleteIndex]?.slice(1)).toEqual([
      remoteTransaction.id,
      remoteTransaction.revision,
    ]);
  });

  it("migrates legacy duplicates and installs a natural-key unique index", async () => {
    const execAsync = jest.fn<Promise<void>, [string]>().mockResolvedValue();
    const database = {
      getFirstAsync: jest.fn().mockResolvedValue({ user_version: 2 }),
      execAsync,
      withTransactionAsync: async (callback: () => Promise<void>) => callback(),
    } as unknown as SQLiteDatabase;

    await runMigrations(database);

    expect(latestMigrationVersion).toBe(3);
    const migrationSql = String(execAsync.mock.calls[0]?.[0]);
    expect(migrationSql).toContain("SELECT MAX(rowid)");
    expect(migrationSql).toContain(
      "transaction_items_package_revision_unique_idx",
    );
  });
});
