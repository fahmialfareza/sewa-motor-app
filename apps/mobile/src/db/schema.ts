import {
  index,
  integer,
  primaryKey,
  sqliteTable,
  text,
  uniqueIndex,
} from "drizzle-orm/sqlite-core";

export const localPackages = sqliteTable("packages_local", {
  id: text("id").primaryKey(),
  revision: integer("revision").notNull(),
  name: text("name").notNull(),
  description: text("description").notNull(),
  unitPrice: integer("unit_price").notNull(),
  accent: text("accent").notNull(),
  active: integer("active", { mode: "boolean" }).notNull(),
  deletedAt: text("deleted_at"),
  updatedAt: text("updated_at").notNull(),
});

export const transactions = sqliteTable(
  "transactions",
  {
    id: text("id").primaryKey(),
    revision: integer("revision").notNull(),
    occurredAt: text("occurred_at").notNull(),
    subtotal: integer("subtotal").notNull(),
    total: integer("total").notNull(),
    originActorId: text("origin_actor_id").notNull(),
    originActorName: text("origin_actor_name").notNull(),
    updatedActorName: text("updated_actor_name").notNull(),
    terminalId: text("terminal_id").notNull(),
    syncState: text("sync_state").notNull(),
    printState: text("print_state").notNull(),
    deletedAt: text("deleted_at"),
    serverUpdatedAt: text("server_updated_at"),
  },
  (table) => [
    index("transactions_occurred_at_idx").on(table.occurredAt),
    index("transactions_sync_state_idx").on(table.syncState),
  ],
);

export const transactionItems = sqliteTable(
  "transaction_items",
  {
    id: text("id").primaryKey(),
    transactionId: text("transaction_id")
      .notNull()
      .references(() => transactions.id),
    revision: integer("revision").notNull(),
    packageId: text("package_id").notNull(),
    packageRevision: integer("package_revision").notNull(),
    name: text("name").notNull(),
    description: text("description").notNull(),
    accent: text("accent").notNull(),
    unitPrice: integer("unit_price").notNull(),
    quantity: integer("quantity").notNull(),
    lineTotal: integer("line_total").notNull(),
  },
  (table) => [
    index("transaction_items_transaction_idx").on(
      table.transactionId,
      table.revision,
    ),
    uniqueIndex("transaction_items_package_revision_unique_idx").on(
      table.transactionId,
      table.revision,
      table.packageId,
      table.packageRevision,
    ),
  ],
);

export const transactionRevisions = sqliteTable(
  "transaction_revisions",
  {
    transactionId: text("transaction_id").notNull(),
    revision: integer("revision").notNull(),
    reason: text("reason"),
    beforeJson: text("before_json"),
    afterJson: text("after_json").notNull(),
    originActorId: text("origin_actor_id").notNull(),
    submittingActorId: text("submitting_actor_id").notNull(),
    submittingActorName: text("submitting_actor_name").notNull(),
    terminalId: text("terminal_id").notNull(),
    clientOccurredAt: text("client_occurred_at").notNull(),
    serverReceivedAt: text("server_received_at"),
  },
  (table) => [primaryKey({ columns: [table.transactionId, table.revision] })],
);

export const auditEvents = sqliteTable("audit_events", {
  id: text("id").primaryKey(),
  kind: text("kind").notNull(),
  aggregateId: text("aggregate_id").notNull(),
  actorId: text("actor_id").notNull(),
  sessionId: text("session_id").notNull(),
  terminalId: text("terminal_id").notNull(),
  payloadJson: text("payload_json").notNull(),
  occurredAt: text("occurred_at").notNull(),
});

export const outboxOperations = sqliteTable(
  "outbox_operations",
  {
    operationId: text("operation_id").primaryKey(),
    aggregate: text("aggregate").notNull(),
    aggregateId: text("aggregate_id").notNull(),
    action: text("action").notNull(),
    baseRevision: integer("base_revision"),
    operationJson: text("operation_json").notNull(),
    signature: text("signature").notNull(),
    state: text("state").notNull(),
    attempts: integer("attempts").notNull(),
    lastError: text("last_error"),
    nextAttemptAt: text("next_attempt_at"),
    occurredAt: text("occurred_at").notNull(),
  },
  (table) => [
    index("outbox_state_order_idx").on(table.state, table.occurredAt),
  ],
);

export const printAttempts = sqliteTable("print_attempts", {
  id: text("id").primaryKey(),
  transactionId: text("transaction_id").notNull(),
  adapter: text("adapter").notNull(),
  isCopy: integer("is_copy", { mode: "boolean" }).notNull(),
  requestedAt: text("requested_at").notNull(),
  completedAt: text("completed_at"),
  result: text("result").notNull(),
  error: text("error"),
});

export const syncConflicts = sqliteTable("sync_conflicts", {
  id: text("id").primaryKey(),
  transactionId: text("transaction_id").notNull(),
  localJson: text("local_json").notNull(),
  serverJson: text("server_json").notNull(),
  createdAt: text("created_at").notNull(),
  resolvedAt: text("resolved_at"),
  resolution: text("resolution"),
});

export const syncedEntities = sqliteTable(
  "synced_entities",
  {
    aggregate: text("aggregate").notNull(),
    aggregateId: text("aggregate_id").notNull(),
    payloadJson: text("payload_json"),
    deletedAt: text("deleted_at"),
    changedAt: text("changed_at").notNull(),
  },
  (table) => [
    primaryKey({ columns: [table.aggregate, table.aggregateId] }),
    index("synced_entities_changed_at_idx").on(
      table.aggregate,
      table.changedAt,
    ),
  ],
);
