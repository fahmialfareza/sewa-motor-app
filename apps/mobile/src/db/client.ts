import { drizzle, type ExpoSQLiteDatabase } from "drizzle-orm/expo-sqlite";
import * as SQLite from "expo-sqlite";

import { getOrCreateDatabaseKey } from "@/security/secure-store";

import { runMigrations } from "./migrations";
import * as schema from "./schema";

export interface DatabaseConnection {
  sqlite: SQLite.SQLiteDatabase;
  orm: ExpoSQLiteDatabase<typeof schema>;
}

let connectionPromise: Promise<DatabaseConnection> | null = null;

export function getDatabase(): Promise<DatabaseConnection> {
  connectionPromise ??= openDatabase();
  return connectionPromise;
}

async function openDatabase(): Promise<DatabaseConnection> {
  const key = await getOrCreateDatabaseKey();
  const sqlite = await SQLite.openDatabaseAsync("sewa-motor-pos.db");

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
}

export async function initializeDatabase(): Promise<void> {
  await getDatabase();
}

export function resetDatabaseSingletonForTests(): void {
  connectionPromise = null;
}
