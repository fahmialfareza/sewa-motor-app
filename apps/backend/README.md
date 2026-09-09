# Telomoyo POS Backend

Go 1.26 and Gin service for the local-first Telomoyo POS app.
PostgreSQL is the source of truth. GORM provides typed, context-aware repository
reads. Explicit pgx transactions remain in the atomic mutation paths that need
serializable isolation, advisory locks, append-only revisions, audit events, and
sync idempotency. Redis is optional and is used only for login rate limiting and
the immutable token-hash-to-session-ID cache; every authorization decision is
revalidated against PostgreSQL.

## Run locally

From the repository root:

```sh
docker compose up -d postgres redis
pnpm --filter @sewa-motor/backend run dev
```

The default Compose environment enables automatic migrations. Without Compose:

```sh
cp apps/backend/.env.example apps/backend/.env
DATABASE_URL='postgres://sewa_motor:sewa_motor@localhost:5432/sewa_motor?sslmode=disable' \
  go run ./apps/backend/cmd/migrate
```

Health endpoints are available at `/api/v1/health/live` and
`/api/v1/health/ready`, with `/healthz` and `/readyz` aliases for hosting
platforms.

## Container targets

The Dockerfile produces separate non-root images so the serving image contains
only the API binary:

```sh
docker build --target api -t sewa-motor-backend:api apps/backend
docker build --target migrate -t sewa-motor-backend:migrate apps/backend
docker build --target bootstrap -t sewa-motor-backend:bootstrap apps/backend
```

The `api` target is the default for a plain `docker build` and is selected
explicitly by Compose. Run migrations as a one-shot step before deploying the
API, then set `AUTO_MIGRATE=false` on the production API container:

```sh
docker run --rm \
  --env DATABASE_URL='postgres://...' \
  sewa-motor-backend:migrate
```

Bootstrap remains a separate, manually authorized one-shot operation. Mount the
secret manifest read-only and ensure it is readable by container UID 65532:

```sh
docker run --rm \
  --env DATABASE_URL='postgres://...' \
  --mount type=bind,src=/secure/path/bootstrap-users.json,dst=/run/secrets/bootstrap-users.json,readonly \
  sewa-motor-backend:bootstrap \
  -manifest /run/secrets/bootstrap-users.json
```

## Database migrations

Runtime migrations are forward-only Go migrations in `migrations/` and receive
the configured `*gorm.DB`. Each version runs atomically under a PostgreSQL
advisory lock. GORM `AutoMigrate` creates the base tables only inside an
unapplied version; deferred composite foreign keys, append-only triggers, and
other PostgreSQL-specific invariants are installed through the same GORM
transaction.

`sqlc/schema.sql` is a code-generation snapshot only. The backend never executes
that SQL file as a migration.

## New Relic observability

Production configuration requires:

```sh
NEW_RELIC_ENABLED=true
NEW_RELIC_APP_NAME=sewa-motor-backend-production
NEW_RELIC_LICENSE_KEY=...
NEW_RELIC_DISTRIBUTED_TRACING_ENABLED=true
NEW_RELIC_LOG_FORWARDING_ENABLED=true
```

The `sewa-motor-backend-production` application name is intentionally retained
as a stable operational identifier so the Telomoyo POS rebrand does not split
APM history, dashboards, or alerts.

The Gin middleware creates a New Relic web transaction for every route. The
request context is propagated through use cases, GORM, pgx, and Redis, with
function-level segments for business and repository work. SQL query parameters
are excluded from pgx telemetry. All API errors, rejected operations inside a
successful sync batch, database/cache failures, and recovered panics are
reported through `NoticeError`.

Logrus emits structured JSON. New Relic's Logrus logs-in-context formatter adds
trace/span correlation and forwards records through the Go agent when log
forwarding is enabled. Do not place session tokens, passwords, terminal private
keys, database URLs, or license keys in log fields.

Every web transaction starts in the fail-closed Production scope, including
health checks, login attempts, and authentication failures. A successful
authenticated request replaces that scope with the session-authorized
`data.mode`, `data.space_id`, and (for Sandbox) `data.generation`. The Sandbox
initializer and janitor also report `data.mode = 'sandbox'`. These attributes
are attached to New Relic transactions, noticed errors, and Logrus records.

Keep business alerts mode-specific. For example, a Production API error
condition and a separate non-revenue Sandbox error view can start from:

```sql
SELECT count(*) FROM TransactionError
WHERE appName = 'sewa-motor-backend-production'
  AND `data.mode` = 'production'
```

```sql
SELECT count(*) FROM TransactionError
WHERE appName = 'sewa-motor-backend-production'
  AND `data.mode` = 'sandbox'
FACET operation
```

Do not add a mode filter to fleet availability, process restart, telemetry
loss, or migration alerts: those failures can occur before a request has a data
scope. A `sandbox.initialize` error is a deployment/startup failure. Monitor
`sandbox.cleanup` separately and configure loss-of-signal on the following log
query while the API fleet is expected to be running. Use 36 hours for the
default 24-hour cleanup interval (and never exceed New Relic's 48-hour
loss-of-signal maximum):

```sql
SELECT count(*) FROM Log
WHERE `data.mode` = 'sandbox'
  AND message = 'sandbox cleanup completed'
```

Before enabling Sandbox, verify that web traffic is classified (the following
must remain zero), Production alerts exclude Sandbox, and the Sandbox error and
cleanup conditions actually receive a test signal:

```sql
SELECT count(*) FROM Transaction
WHERE appName = 'sewa-motor-backend-production'
  AND transactionType = 'Web'
  AND `data.mode` IS NULL
```

## Production-hosted Sandbox Mode

Sandbox Mode is an isolated data space inside the production service, not a
replacement deployment environment. It is disabled by default:

```sh
SANDBOX_ENABLED=false
SANDBOX_RETENTION_DAYS=30
SANDBOX_QRIS_AMOUNT=1000
SANDBOX_CLEANUP_INTERVAL=24h
```

`SANDBOX_QRIS_AMOUNT` is a fixed safety invariant; startup rejects any value
other than Rp1.000. The client generates the Sandbox QR from the real configured
merchant payload, warns that the transfer is real, and marks every sandbox
screen, receipt, and export as test output.

Roll out in this order:

1. Back up PostgreSQL and apply the forward-only GORM migration while
   `SANDBOX_ENABLED=false`. This migration only adds and backfills the data-space
   schema; it does not create, clone, or seed a Sandbox generation.
2. Deploy the scope-aware backend with Sandbox still disabled.
3. Release the compatible mobile build, then verify production dashboard,
   history, exports, and sync remain in the production data space.
4. Confirm that no pre-data-space backend replicas remain. Filter New Relic
   production alerts by `data.mode = 'production'` and create a separate
   Sandbox cleanup/error view.
5. Set `SANDBOX_ENABLED=true` and restart the API. Before accepting traffic,
   each replica runs an advisory-locked, idempotent activation; the first
   replica creates the generation and clones the then-current production
   packages, while the others reuse it. Any activation failure prevents that
   replica from starting.

`GET /sandbox/status` remains usable while the feature is disabled. It returns
`enabled: false`; `dataSpaceId` and `generation` are `null` only when Sandbox
has never been activated. A previously activated generation remains identified
while entry is disabled, so clients can recognize a rollback without assuming
that retained data disappeared.

Switching mode is online-only, available to every signed-in staff member, and
rotates the session into the selected data space. Only a production-mode
superadmin may reset Sandbox. Reset advances the sandbox generation; the
retired generation stays append-only until its configured retention boundary
(30 days by default), after which cleanup removes it without affecting
production readiness. Set `SANDBOX_ENABLED=false` to stop new sandbox entry
while preserving production service; retired Sandbox evidence remains subject
to that retention policy.

### Backup and restore boundary

Sandbox rows share the Production PostgreSQL cluster. Provider snapshots,
continuous archiving, and PITR are physical recovery mechanisms and therefore
include both modes; PostgreSQL cannot apply a row predicate to them. Treat those
recovery copies as mixed-mode data with the same encryption and access controls
as Production. Application cleanup does not retroactively remove rows from old
snapshots, so align provider snapshot retention with the approved Sandbox-data
policy. If policy forbids Sandbox rows in every recovery copy, do not enable a
production-hosted Sandbox in that cluster.

Use a production-only logical artifact for long-lived or externally transferred
backups. `pg_dump` has no row-level `WHERE` option, so `--exclude-table-data`
cannot safely express this boundary. The operator procedure is:

1. Capture one consistent full recovery point and restore it into an isolated,
   disposable scratch database. Never run the sanitization below against the
   live database, and never point an API replica at the scratch database.
2. As the schema owner, retire and remove every Sandbox generation in the
   scratch copy using the same guarded append-only exception as the janitor:

   ```sql
   BEGIN;

   UPDATE data_spaces
   SET status = 'retired',
       retired_at = COALESCE(retired_at, transaction_timestamp()),
       purge_after = LEAST(
           COALESCE(purge_after, transaction_timestamp()),
           transaction_timestamp()
       ),
       purged_at = NULL
   WHERE mode = 'sandbox';

   DO $sanitize$
   DECLARE
       sandbox_ids uuid[];
       sandbox_id uuid;
   BEGIN
       SELECT COALESCE(array_agg(id), ARRAY[]::uuid[])
       INTO sandbox_ids
       FROM data_spaces
       WHERE mode = 'sandbox';

       FOREACH sandbox_id IN ARRAY sandbox_ids LOOP
           PERFORM set_config(
               'app.sandbox_purge_data_space_id',
               sandbox_id::text,
               true
           );
           DELETE FROM idempotency_records WHERE data_space_id = sandbox_id;
           DELETE FROM sync_changes WHERE data_space_id = sandbox_id;
           DELETE FROM audit_events WHERE data_space_id = sandbox_id;
           DELETE FROM print_attempts WHERE data_space_id = sandbox_id;
           DELETE FROM transaction_items WHERE data_space_id = sandbox_id;
           DELETE FROM transaction_revisions WHERE data_space_id = sandbox_id;
           DELETE FROM transactions WHERE data_space_id = sandbox_id;
           DELETE FROM package_revisions WHERE data_space_id = sandbox_id;
           DELETE FROM packages WHERE data_space_id = sandbox_id;
           DELETE FROM sessions WHERE data_space_id = sandbox_id;
           DELETE FROM data_spaces WHERE id = sandbox_id;
       END LOOP;
   END
   $sanitize$;

   COMMIT;
   ```

3. Run this verification query in the scratch copy. It must return no rows:

   ```sql
   WITH unexpected(table_name, row_count) AS (
       SELECT 'production_data_space',
              CASE WHEN count(*) = 1 THEN 0 ELSE 1 END
       FROM data_spaces
       WHERE id = '00000000-0000-4000-8000-000000000100'::uuid
         AND mode = 'production'
         AND generation = 1
         AND status = 'active'
       UNION ALL SELECT 'data_spaces', count(*) FROM data_spaces
       WHERE id <> '00000000-0000-4000-8000-000000000100'::uuid
          OR mode <> 'production'
       UNION ALL SELECT 'sessions', count(*) FROM sessions
       WHERE data_space_id <> '00000000-0000-4000-8000-000000000100'::uuid
       UNION ALL SELECT 'packages', count(*) FROM packages
       WHERE data_space_id <> '00000000-0000-4000-8000-000000000100'::uuid
       UNION ALL SELECT 'package_revisions', count(*) FROM package_revisions
       WHERE data_space_id <> '00000000-0000-4000-8000-000000000100'::uuid
       UNION ALL SELECT 'transactions', count(*) FROM transactions
       WHERE data_space_id <> '00000000-0000-4000-8000-000000000100'::uuid
       UNION ALL SELECT 'transaction_revisions', count(*) FROM transaction_revisions
       WHERE data_space_id <> '00000000-0000-4000-8000-000000000100'::uuid
       UNION ALL SELECT 'transaction_items', count(*) FROM transaction_items
       WHERE data_space_id <> '00000000-0000-4000-8000-000000000100'::uuid
       UNION ALL SELECT 'print_attempts', count(*) FROM print_attempts
       WHERE data_space_id <> '00000000-0000-4000-8000-000000000100'::uuid
       UNION ALL SELECT 'audit_events', count(*) FROM audit_events
       WHERE data_space_id <> '00000000-0000-4000-8000-000000000100'::uuid
       UNION ALL SELECT 'sync_changes', count(*) FROM sync_changes
       WHERE data_space_id <> '00000000-0000-4000-8000-000000000100'::uuid
       UNION ALL SELECT 'idempotency_records', count(*) FROM idempotency_records
       WHERE data_space_id <> '00000000-0000-4000-8000-000000000100'::uuid
   )
   SELECT * FROM unexpected WHERE row_count <> 0;
   ```

4. Create and restore-test the logical `pg_dump` artifact from that verified
   scratch database, record its checksum and recovery point, then destroy the
   scratch database. On restore, repeat the query above before exposing the
   database to an API replica.

## Bootstrap users

Bootstrap requires an uncommitted JSON secret containing exactly one
superadmin and seven admins. All passwords are temporary and every account is
forced to change its password before accessing POS features.

```json
{
  "users": [
    {
      "fullName": "Pemilik",
      "username": "pemilik",
      "role": "superadmin",
      "temporaryPassword": "replace-with-a-secret"
    }
  ]
}
```

The real manifest must contain eight entries and passwords of at least 12
characters. Keep it outside Git, then run:

```sh
DATABASE_URL='postgres://...' \
  go run ./apps/backend/cmd/bootstrap -manifest /secure/path/bootstrap-users.json
```

The command is idempotent by username and never silently resets an existing
password.

### Sample development superadmin

For local development, create one sample superadmin without weakening the
production eight-user bootstrap contract:

```sh
pnpm seed:superadmin
```

The command reads `apps/backend/.env`, requires `APP_ENV=development`, applies
pending GORM migrations, and creates `superadmin` / `Penyok`. If
`DEV_SUPERADMIN_PASSWORD` is empty, the development-only temporary password is
`superadmin123`. The seed is idempotent and never resets an existing password.
The account must change its password after its first login.

To explicitly replace the password of an existing sample account, revoke all
of its active sessions, and require another password change, run:

```sh
pnpm seed:superadmin --reset-password
```

The reset is transactional and records both an audit event and a sync change.
It is only available when `APP_ENV=development`; ordinary seed reruns remain
non-mutating.

## Generated contracts

The authoritative OpenAPI 3.1 contract is at `../../api/openapi.yaml`. Code
generation creates a deterministic OpenAPI 3.0 compatibility view for
oapi-codegen and generates sqlc query types:

```sh
pnpm --filter @sewa-motor/backend run generate
pnpm --filter @sewa-motor/backend run generate:check
```

## Signed synchronization

Each outbox operation is signed with its enrolled terminal Ed25519 key. The
signed object has exactly these fields:

```text
operationId, aggregate, aggregateId, action, baseRevision,
originSessionId, originActorId, terminalId, occurredAt, payload
```

`baseRevision` is always present and is `null` for creates. `occurredAt` is UTC
with exactly millisecond precision. The object is canonicalized using RFC 8785
and the signature is standard Base64. The `signature` property itself is not
signed.

Business mutation, audit event, sync change, and stored idempotency result are
committed in one PostgreSQL transaction. Retries with the same operation ID
replay the original success or deterministic conflict; reuse with another
payload is rejected.

## Verification

```sh
pnpm --filter @sewa-motor/backend run lint
pnpm --filter @sewa-motor/backend run test
pnpm --filter @sewa-motor/backend run build
```

The focused test suite covers Argon2id and opaque sessions, RFC 8785/Ed25519
golden vectors, transaction snapshot contracts, RBAC safeguards, sync replay
and conflict behavior, export artifacts, bootstrap validation, and HTTP
envelopes.
