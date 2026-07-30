# Sewa Motor POS Backend

Go 1.26 and Gin service for the local-first Sewa Motor point-of-sale app.
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
