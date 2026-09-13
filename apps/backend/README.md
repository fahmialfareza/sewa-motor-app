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

## Organization accounts and tenant scope

Exactly two global roles are authoritative: `admin` and `superadmin`. Every
active account may enter every active tenant without invitation or membership
approval; historical membership links remain for attribution and constraints.
Business APIs still derive tenant/data-space scope from the session, and
cross-tenant identifiers return `404`. Account context has no business access.

Account-context `/management/tenants`, `/management/users`, and
`/management/audit` require a global Superadmin. New tenants are directly active
with an empty catalog and initial receipt identity. Provisioning controls only
creation; names can change independently of receipt identity, IDs/slugs cannot,
and tenant deletion is unavailable. Global staff creation/recovery requires a
temporary-password change. Role changes, deactivation, and recovery revoke all
account sessions; self-demotion/deactivation and last-Superadmin removal are
blocked. Old invitation and membership-mutation routes fail explicitly instead
of becoming global account mutations.

Only a Production-mode Superadmin can enroll/revoke a terminal or change tenant
QRIS/receipt identity. Admins operate valid existing enrollments on shared
devices and retain ownership-limited correction/payment permissions. Sessions,
terminal keys, QRIS history, and signed queues stay tenant-scoped. See the
[operations guide](../../docs/multi-tenant-operations.md) for operator recovery,
safe switching/quarantine, migration rehearsal, and release gates.

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

Authenticated tenant requests carry session-authorized `auth.context`,
`tenant.id`, `data.mode`, `data.space_id`, generation, and payment-policy
attributes. Account/public requests do not claim a business data space. Sandbox
initializer and janitor work is separately classified. These attributes apply
to New Relic transactions, noticed errors, and correlated Logrus records.

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
  AND `auth.context` = 'tenant'
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

`SANDBOX_QRIS_AMOUNT=1000` is deprecated but accepted with a warning during
transition; it does not override new session policy. Protocol-v3 Sandbox QRIS
uses the full Sandbox package-price total, independently of Production prices,
and the tenant's real merchant payload. Transfers are real and require manual
payment confirmation/reconciliation. Screens, receipts, and exports remain
marked as test output.

Legacy sessions retain immutable `sandboxQrisPolicy=fixed_1000`. Signed mutation
amounts are derived from the validated origin session, not the submitting app.
Existing payments, QR codes, and reprints retain their stored amount. The new
client drains the old outbox online before `/auth/upgrade-session` negotiates
protocol 3 with `transaction_total`; failed upgrades must block new Sandbox
creates/corrections. Never rewrite or re-sign historical queues. Follow the
[internal operations guide](../../docs/multi-tenant-operations.md) for global-role
migration, compatible rollout, and rollback boundaries.

For first-time Sandbox activation, use the following sequence. An existing
installation upgrading to global roles/full-value Sandbox must first follow
the internal-organization rollout in the operations guide, including stopping
all incompatible replicas before applying migration `000006`.

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

Infrastructure backups are operator-only recovery copies of the full shared
database: all tenants, Production and Sandbox, credentials, and audit history.
Encrypt and restrict snapshots, continuous archives, dumps, and restore
environments. Never distribute raw snapshots to staff; report exports are not
database backups. Tenant-specific backup delivery is outside this release.

Rehearse recovery in an isolated disposable database that no serving API or
device can reach. Compare tenant/account counts, immutable IDs, revisions,
payment amounts, QRIS bindings, sessions, and audit history against the captured
recovery point before testing migrations there. Keep recovered credentials
protected and prevent the rehearsal service from forwarding logs or acting on
real merchant/printer integrations. Never sanitize the live database or use
single-tenant filtering assumptions to remove other tenants' data.

Sandbox cleanup does not retroactively remove evidence from retained recovery
copies. Apply the operator's approved backup retention independently of the
30-day retired-generation cleanup. Only release a restore after its tenant and
payment-policy compatibility has been verified by an authorized operator.

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
