import { drizzle, type ExpoSQLiteDatabase } from "drizzle-orm/expo-sqlite";
import * as SQLite from "expo-sqlite";

import type { DataMode, Session } from "@/domain/types";
import { activeDataMode } from "@/mode/mode-store";
import { getOrCreateDatabaseKey } from "@/security/secure-store";

import { runMigrations } from "./migrations";
import * as schema from "./schema";

export interface DatabaseConnection {
  sqlite: SQLite.SQLiteDatabase;
  orm: ExpoSQLiteDatabase<typeof schema>;
}

export const databaseNames: Record<DataMode, string> = {
  production: "sewa-motor-pos.db",
  sandbox: "sewa-motor-pos-sandbox.db",
};

const connectionPromises: Partial<
  Record<DataMode, Promise<DatabaseConnection>>
> = {};

export function getDatabase(
  mode: DataMode = activeDataMode(),
): Promise<DatabaseConnection> {
  const existing = connectionPromises[mode];
  if (existing) return existing;

  let opening: Promise<DatabaseConnection>;
  opening = openDatabase(mode).catch((error: unknown) => {
    if (connectionPromises[mode] === opening) {
      delete connectionPromises[mode];
    }
    throw error;
  });
  connectionPromises[mode] = opening;
  return opening;
}

async function openDatabase(mode: DataMode): Promise<DatabaseConnection> {
  const key = await getOrCreateDatabaseKey(mode);
  const sqlite = await SQLite.openDatabaseAsync(databaseNames[mode]);
  try {
    // The generated key is hexadecimal only, so it cannot escape this pragma.
    await sqlite.execAsync(`
      PRAGMA key = "x'${key}'";
      PRAGMA foreign_keys = ON;
      PRAGMA journal_mode = WAL;
      PRAGMA busy_timeout = 5000;
    `);
    await runMigrations(sqlite);

    return {
      sqlite,
      orm: drizzle(sqlite, { schema }),
    };
  } catch (error) {
    await sqlite.closeAsync().catch(() => undefined);
    throw error;
  }
}

export async function initializeDatabase(
  mode: DataMode = activeDataMode(),
): Promise<void> {
  await getDatabase(mode);
}

export async function prepareDatabaseForSession(
  session: Pick<Session, "dataMode" | "sandboxGeneration">,
): Promise<void> {
  if (session.dataMode === "production") {
    await initializeDatabase("production");
    return;
  }
  if (!session.sandboxGeneration) {
    throw new Error("Generasi Mode Uji pada sesi tidak valid.");
  }

  let connection = await getDatabase("sandbox");
  const metadata = await connection.sqlite.getFirstAsync<{
    generation: number | null;
  }>("SELECT generation FROM sync_metadata WHERE singleton = 1");
  if (metadata?.generation === session.sandboxGeneration) return;

  await clearLocalDatabase("sandbox");
  connection = await getDatabase("sandbox");
  await connection.sqlite.withTransactionAsync(async () => {
    await connection.sqlite.execAsync(`
      DELETE FROM sync_conflicts;
      DELETE FROM print_attempts;
      DELETE FROM outbox_operations;
      DELETE FROM audit_events;
      DELETE FROM transaction_revisions;
      DELETE FROM transaction_items;
      DELETE FROM transactions;
      DELETE FROM synced_entities;
      DELETE FROM packages_local;
    `);
    await connection.sqlite.runAsync(
      `UPDATE sync_metadata
       SET generation = ?, cursor = NULL, status = 'idle',
           last_synced_at = NULL, last_error = NULL
       WHERE singleton = 1`,
      session.sandboxGeneration,
    );
  });
}

export async function clearLocalDatabase(mode: DataMode): Promise<void> {
  const existing = connectionPromises[mode];
  delete connectionPromises[mode];
  let failure: unknown;
  if (existing) {
    try {
      const connection = await existing;
      await connection.sqlite.closeAsync();
    } catch (error) {
      failure = error;
    }
  }
  try {
    await SQLite.deleteDatabaseAsync(databaseNames[mode]);
  } catch (error) {
    failure ??= error;
  }
  if (failure) throw failure;
}

export function resetDatabaseSingletonForTests(): void {
  delete connectionPromises.production;
  delete connectionPromises.sandbox;
}
