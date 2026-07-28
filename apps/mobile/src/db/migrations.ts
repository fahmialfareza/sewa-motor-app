import type { SQLiteDatabase } from "expo-sqlite";

interface Migration {
  version: number;
  name: string;
  sql: string;
}

const migrations: Migration[] = [
  {
    version: 1,
    name: "local_first_foundation",
    sql: `
      CREATE TABLE IF NOT EXISTS packages_local (
        id TEXT PRIMARY KEY NOT NULL,
        revision INTEGER NOT NULL,
        name TEXT NOT NULL,
        description TEXT NOT NULL,
        unit_price INTEGER NOT NULL CHECK(unit_price >= 0),
        accent TEXT NOT NULL,
        active INTEGER NOT NULL DEFAULT 1,
        deleted_at TEXT,
        updated_at TEXT NOT NULL
      );

      CREATE TABLE IF NOT EXISTS transactions (
        id TEXT PRIMARY KEY NOT NULL,
        revision INTEGER NOT NULL DEFAULT 1,
        occurred_at TEXT NOT NULL,
        subtotal INTEGER NOT NULL CHECK(subtotal >= 0),
        total INTEGER NOT NULL CHECK(total >= 0),
        origin_actor_id TEXT NOT NULL,
        origin_actor_name TEXT NOT NULL,
        updated_actor_name TEXT NOT NULL,
        terminal_id TEXT NOT NULL,
        sync_state TEXT NOT NULL,
        print_state TEXT NOT NULL,
        deleted_at TEXT,
        server_updated_at TEXT
      );
      CREATE INDEX IF NOT EXISTS transactions_occurred_at_idx
        ON transactions(occurred_at DESC);
      CREATE INDEX IF NOT EXISTS transactions_sync_state_idx
        ON transactions(sync_state);

      CREATE TABLE IF NOT EXISTS transaction_items (
        id TEXT PRIMARY KEY NOT NULL,
        transaction_id TEXT NOT NULL REFERENCES transactions(id),
        revision INTEGER NOT NULL,
        package_id TEXT NOT NULL,
        package_revision INTEGER NOT NULL,
        name TEXT NOT NULL,
        description TEXT NOT NULL,
        accent TEXT NOT NULL DEFAULT 'primary',
        unit_price INTEGER NOT NULL,
        quantity INTEGER NOT NULL CHECK(quantity BETWEEN 1 AND 999),
        line_total INTEGER NOT NULL
      );
      CREATE INDEX IF NOT EXISTS transaction_items_transaction_idx
        ON transaction_items(transaction_id, revision);

      CREATE TABLE IF NOT EXISTS transaction_revisions (
        transaction_id TEXT NOT NULL,
        revision INTEGER NOT NULL,
        reason TEXT,
        before_json TEXT,
        after_json TEXT NOT NULL,
        origin_actor_id TEXT NOT NULL,
        submitting_actor_id TEXT NOT NULL,
        submitting_actor_name TEXT NOT NULL,
        terminal_id TEXT NOT NULL,
        client_occurred_at TEXT NOT NULL,
        server_received_at TEXT,
        PRIMARY KEY(transaction_id, revision)
      );

      CREATE TABLE IF NOT EXISTS audit_events (
        id TEXT PRIMARY KEY NOT NULL,
        kind TEXT NOT NULL,
        aggregate_id TEXT NOT NULL,
        actor_id TEXT NOT NULL,
        session_id TEXT NOT NULL,
        terminal_id TEXT NOT NULL,
        payload_json TEXT NOT NULL,
        occurred_at TEXT NOT NULL
      );

      CREATE TABLE IF NOT EXISTS outbox_operations (
        operation_id TEXT PRIMARY KEY NOT NULL,
        aggregate TEXT NOT NULL,
        aggregate_id TEXT NOT NULL,
        action TEXT NOT NULL,
        base_revision INTEGER,
        operation_json TEXT NOT NULL,
        signature TEXT NOT NULL,
        state TEXT NOT NULL DEFAULT 'pending',
        attempts INTEGER NOT NULL DEFAULT 0,
        last_error TEXT,
        next_attempt_at TEXT,
        occurred_at TEXT NOT NULL
      );
      CREATE INDEX IF NOT EXISTS outbox_state_order_idx
        ON outbox_operations(state, occurred_at);

      CREATE TABLE IF NOT EXISTS print_attempts (
        id TEXT PRIMARY KEY NOT NULL,
        transaction_id TEXT NOT NULL,
        adapter TEXT NOT NULL,
        is_copy INTEGER NOT NULL DEFAULT 0,
        requested_at TEXT NOT NULL,
        completed_at TEXT,
        result TEXT NOT NULL,
        error TEXT
      );

      CREATE TABLE IF NOT EXISTS sync_conflicts (
        id TEXT PRIMARY KEY NOT NULL,
        transaction_id TEXT NOT NULL,
        local_json TEXT NOT NULL,
        server_json TEXT NOT NULL,
        created_at TEXT NOT NULL,
        resolved_at TEXT,
        resolution TEXT
      );

      CREATE TABLE IF NOT EXISTS sync_metadata (
        singleton INTEGER PRIMARY KEY NOT NULL CHECK(singleton = 1),
        cursor TEXT,
        status TEXT NOT NULL DEFAULT 'idle',
        last_synced_at TEXT,
        last_error TEXT
      );
      INSERT OR IGNORE INTO sync_metadata(singleton, status)
        VALUES(1, 'idle');

      INSERT OR IGNORE INTO packages_local(
        id, revision, name, description, unit_price, accent, active, updated_at
      ) VALUES
        ('00000000-0000-4000-8000-000000000001', 1, 'Paket Standar', 'Paket sewa motor standar', 70000, 'standard', 1, '2026-01-01T00:00:00.000Z'),
        ('00000000-0000-4000-8000-000000000002', 1, 'Paket Sunrise', 'Paket sewa motor sunrise', 100000, 'sunrise', 1, '2026-01-01T00:00:00.000Z');
    `,
  },
  {
    version: 2,
    name: "cache_all_sync_aggregates",
    sql: `
      CREATE TABLE IF NOT EXISTS synced_entities (
        aggregate TEXT NOT NULL,
        aggregate_id TEXT NOT NULL,
        payload_json TEXT,
        deleted_at TEXT,
        changed_at TEXT NOT NULL,
        PRIMARY KEY(aggregate, aggregate_id)
      );
      CREATE INDEX IF NOT EXISTS synced_entities_changed_at_idx
        ON synced_entities(aggregate, changed_at DESC);
    `,
  },
  {
    version: 3,
    name: "deduplicate_transaction_items",
    sql: `
      DELETE FROM transaction_items
      WHERE rowid NOT IN (
        SELECT MAX(rowid)
        FROM transaction_items
        GROUP BY transaction_id, revision, package_id, package_revision
      );

      CREATE UNIQUE INDEX IF NOT EXISTS transaction_items_package_revision_unique_idx
        ON transaction_items(
          transaction_id,
          revision,
          package_id,
          package_revision
        );
    `,
  },
];

export async function runMigrations(database: SQLiteDatabase): Promise<void> {
  const row = await database.getFirstAsync<{ user_version: number }>(
    "PRAGMA user_version",
  );
  let currentVersion = row?.user_version ?? 0;

  for (const migration of migrations) {
    if (migration.version <= currentVersion) continue;
    await database.withTransactionAsync(async () => {
      await database.execAsync(migration.sql);
      await database.execAsync(`PRAGMA user_version = ${migration.version}`);
    });
    currentVersion = migration.version;
  }
}

export const latestMigrationVersion =
  migrations[migrations.length - 1]?.version ?? 0;
