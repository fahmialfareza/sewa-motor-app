# Sewa Motor POS

Android-first, local-first point of sale for one motorcycle-rental store with
multiple physical terminals. The mobile app remains usable through connectivity
loss; PostgreSQL is the durable source of truth and Redis is disposable cache and
rate-limit infrastructure.

This repository is the v1 implementation foundation. Hardware-specific printer
acceptance still depends on the target MPOS model and vendor SDK.

## V1 product boundary

V1 records package quantities, immutable price/name snapshots, a cash-or-QRIS
payment method, and revision-bound pending/success/failed payment state. QRIS
compatibility mode derives an amount-specific code from a configured static
merchant payload; settlement is confirmed manually until an official payment
provider callback is integrated. V1 does not model motorcycle inventory,
customers, rental schedules, tax, shifts, photos, targets, or cancellations.

- Paket Standar: Rp70.000, canonical ID
  `00000000-0000-0000-0000-000000000001`
- Paket Sunrise: Rp100.000, canonical ID
  `00000000-0000-0000-0000-000000000002`
- Transaction persistence/API IDs are uppercase 26-character ULIDs. The UI and
  receipt add `TRX-`; that prefix is never stored.
- Money is whole rupiah and reporting uses `Asia/Jakarta`; weeks begin Monday.
- Admins can create/read transactions, correct and confirm payment for their own
  transactions, view statistics/exports, read packages, and change their own
  password.
- Superadmins additionally manage users/packages and perform online-only
  transaction deletion, and may correct or confirm payment for every
  transaction. Self-demotion/deactivation/deletion and removal of the final
  active superadmin are forbidden.

## Repository

```text
apps/
  backend/       Go 1.26, Gin, GORM/pgx/sqlc, PostgreSQL, Redis, New Relic
  mobile/        Expo SDK 57 / React Native 0.86 Android app with Zustand
api/
  openapi.yaml   OpenAPI 3.1 source of truth
packages/
  api-client/    generated TypeScript paths plus typed fetch transport
  eslint-config/
  typescript-config/
compose.yaml     local PostgreSQL, Redis, and optional backend profile
```

The backend follows domain/use-case/port/adapter boundaries. GORM provides typed
repository reads and forward-only, versioned database migrations; explicit pgx
transactions retain exact control over serializable writes, advisory locks,
revisions, audit events, and sync idempotency. The mobile app uses Zustand for
live state, encrypted SQLite as its UI read model, SecureStore for secrets, a
signed FIFO outbox, and cursor-based pull synchronization.

## Prerequisites

- Node.js 22
- pnpm 9.0.0
- Go 1.26
- Docker with Compose
- JDK 17 and Android SDK for a mobile development build
- EAS access for signed preview/production Android artifacts

Expo Go cannot load SQLCipher or the local Kotlin printer module. Use an Expo
development build from the beginning.

## Local setup

```sh
cp .env.example .env
pnpm install
pnpm infra:up
pnpm dev
```

`pnpm infra:up` starts only PostgreSQL and Redis. To build and run the backend
container as well:

```sh
pnpm infra:app
```

The default passwords and peppers are development-only. Never reuse them in a
deployed environment. Bootstrap reads an uncommitted manifest from `secrets/`;
every initial/reset password must be temporary and changed on first login.

New Relic is disabled locally by default. To enable it, set
`NEW_RELIC_ENABLED=true`, `NEW_RELIC_LICENSE_KEY`, and a deployment-specific
`NEW_RELIC_APP_NAME`. Never commit the license key.

The Android emulator reaches a host backend through
`http://10.0.2.2:8080/api/v1`. A physical MPOS must use an address reachable from
that device.

## Common commands

| Command                                            | Purpose                                     |
| -------------------------------------------------- | ------------------------------------------- |
| `pnpm dev`                                         | Run workspace development tasks             |
| `pnpm build`                                       | Build every workspace                       |
| `pnpm lint`                                        | Lint TypeScript and vet Go                  |
| `pnpm check-types`                                 | Check TypeScript and compile-check Go       |
| `pnpm test`                                        | Run workspace tests                         |
| `pnpm generate`                                    | Regenerate OpenAPI/sqlc artifacts           |
| `pnpm generate:check`                              | Fail when committed generated code is stale |
| `pnpm --filter @sewa-motor/mobile native:prebuild` | Generate the Android native project         |
| `pnpm openapi:lint`                                | Validate the public contract                |
| `pnpm compose:config`                              | Validate Compose interpolation and syntax   |
| `pnpm validate:foundation`                         | Check OpenAPI, generated drift, and Compose |
| `pnpm format:check`                                | Check repository formatting                 |

## API and synchronization contract

[`api/openapi.yaml`](api/openapi.yaml) is authoritative. JSON success responses
use `{data, meta}` and errors use
`{error: {code, message, details, requestId}}`; binary XLSX/PDF downloads are the
only exception.

The contract covers authentication/profile, users, packages, transactions and
revisions, print attempts, dashboard statistics, exports, terminal enrollment,
sync push/pull, and health/readiness. Do not hand-edit
`packages/api-client/src/generated/schema.ts`.

A signed sync operation has a UUID `operationId`, explicit `aggregateId`, origin
session/actor, server terminal UUID, client time, payload, and Base64 Ed25519
signature. The signature covers exactly `SyncMutationSignedBody` after RFC 8785
canonicalization; `baseRevision` must be present as `null` for creates.
Duplicates replay the stored result. Stale corrections return a revision conflict
with local and server snapshots.

## Infrastructure and release inputs

Compose is for development and CI. Production requires managed credentials,
TLS, backups, restore rehearsal, monitoring, and a deliberate GORM migration
step. Redis loss must only affect cache/rate limiting, never correctness.

The backend Dockerfile exposes separate `migrate`, `bootstrap`, and `api`
targets. Production should run the migration image as a one-shot pre-deploy
step, run bootstrap only when explicitly provisioning initial users, and deploy
the API image without either administrative binary.

The following inputs remain intentionally unset:

- Android application ID and Expo/Play owners
- backend hosting/domain and TLS termination
- initial one-superadmin/seven-admin secret manifest
- MPOS model/OS, paper widths, Bluetooth capabilities
- vendor printer SDK/AAR/JAR and license
- internal tester list

Production release requires a Play Internal Testing AAB, physical acceptance on
each supported MPOS/printer combination, successful backup restore, and no
unexplained transaction loss or duplication.
