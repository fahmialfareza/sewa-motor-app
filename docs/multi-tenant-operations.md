# Multi-tenant operations and release acceptance

## Scope and defaults

Telomoyo POS uses one PostgreSQL database with tenant-owned shared tables.
Tenant UUID `00000000-0000-4000-8000-000000000200` is the migrated **Telomoyo**
business. Production space `00000000-0000-4000-8000-000000000100` and all existing
entity IDs remain unchanged. New businesses start with an empty catalog, IDR,
`Asia/Jakarta`, and shared app branding. Custom logos, billing, tenant deletion,
branches, and cross-business reporting are outside this release.

```dotenv
TENANT_PROVISIONING_ENABLED=false
SANDBOX_ENABLED=false
SANDBOX_QRIS_AMOUNT=1000
SANDBOX_RETENTION_DAYS=30
SANDBOX_CLEANUP_INTERVAL=24h
```

The provisioning flag stops new business creation; it is not an authorization
bypass, a switch back to single tenancy, or a substitute for suspending a tenant.
Sandbox is independently enabled for the deployment, with an independent active
generation and reset/retention lifecycle in each tenant. Reset does not touch
other tenants. Sandbox QRIS charges are real Rp1.000 transfers requiring manual
merchant reconciliation.

## Safe rollout

1. Capture and verify an operator-controlled full database recovery point.
   Exercise migration `000005_tenants` against a populated disposable restore;
   compare existing IDs, totals, historical revisions, and origin-session proof.
2. Keep provisioning disabled. Stop **all old backend replicas** before the
   final membership backfill and tenant-aware deployment. Old global queries
   cannot safely coexist with additional tenants. This migration is not a
   rolling-deployment coexistence guarantee.
3. Run the forward-only GORM migration once, using the `migrate` container target.
   Start only tenant-aware replicas, preferably with `AUTO_MIGRATE=false` after
   the one-shot migration. Verify production readiness and existing Telomoyo
   sessions, numeric cursors, signed outboxes, reports, and payment bindings.
4. Release the compatible mobile build. Verify the explicit legacy storage/key
   mapping for Telomoyo on an installed device with pending offline work. Never
   reinstall or delete app data as a migration step. Account/platform contexts
   must not open a business database.
5. Explicitly assign the first platform administrator using the audited operator
   command below. This does not grant a membership or promote existing tenant
   superadmins. Confirm platform management cannot query business APIs.
6. Confirm no old replicas remain and complete the two-tenant acceptance checks
   below. Enable `TENANT_PROVISIONING_ENABLED=true`, restart the compatible API,
   and provision the first additional business from platform management.

After additional tenants exist, rollback must **retain tenant-aware authorization**
and database scope checks. Disabling provisioning only stops new tenant creation;
do not restore an older single-tenant binary against the shared database.
`SANDBOX_ENABLED=false` remains the independent Sandbox entry rollback switch.

## Audited global account operations

These are operator-only maintenance commands, not tenant-user management APIs.
They require an existing active account and an explicit target, operator identity,
action, and reason. Obtain account-owner authorization out of band for recovery.
Protect the host/container database credentials and retain infrastructure access
audit alongside the command's durable `platform_audit_events` record. The
`--operator` value is supplied by the responsible operator, not an authentication
mechanism. Never put a password, invitation code, or QRIS payload in `--reason`.

From the repository root:

```sh
pnpm --filter @sewa-motor/backend account:admin \
  --action grant-platform --username approved.operator \
  --operator on-call@example.test --reason 'Approved initial platform assignment'
```

Use `--action revoke-platform` with the same explicit fields to remove platform
authority. It revokes platform sessions while retaining tenant memberships and
account/tenant sessions. Granting platform authority never creates business
membership. Account-owner password recovery uses `--action reset-password`;
the temporary password is accepted **only on standard input**, never an argument
or environment variable. For example, on zsh, enter it without terminal echo:

```sh
read -rs 'temporary_password?Temporary password: '
printf '\n'
printf '%s' "$temporary_password" | pnpm --filter @sewa-motor/backend account:admin \
  --action reset-password --username approved.account \
  --operator on-call@example.test --reason 'Verified account-owner recovery'
unset temporary_password
```

Use a unique temporary password of 8–256 bytes. Recovery changes only the global
credential, sets `must_change_password=true`, and revokes **all** existing account,
platform, and tenant sessions. It does not rename the person, alter membership
roles, rewrite signed queues, or grant authority over another business. The user
must sign in online and change the temporary password. Quarantined offline
evidence is retained and must pass exact origin authorization revalidation before
replay; password recovery does not authorize replay under another account.

The separate non-root container target is built without putting operator tooling
inside the serving image:

```sh
docker build --target account-admin -t telomoyo-pos:account-admin apps/backend
docker run --rm --env DATABASE_URL \
  telomoyo-pos:account-admin --action grant-platform --username approved.operator \
  --operator on-call@example.test --reason 'Approved initial platform assignment'
```

For a reset, pipe the password from a protected input source and add `docker run
--interactive`; do not mount application data or expose the command as an HTTP
service. The command never runs migrations automatically.

## Invitations, sessions, and device recovery

New tenants remain `pending_setup` until the initial-owner invitation is consumed
atomically with membership creation. Codes expire after seven days, are stored
hashed, and are exposed only when issued. Reissuing an owner code invalidates the
previous one. Existing users sign in before accepting; new accounts register
through a valid invitation. After activation, only the business's superadmins
manage invitations/memberships. A platform admin cannot self-enroll in an existing
tenant through platform management.

Protocol-v2 login first establishes an account context. The app then selects the
sole available business's Production space or shows a chooser for multiple
businesses/platform management. Each context/mode exchange creates a new session
and revokes the previous session without changing its historical tenant, actor,
membership, or data-space identity. Ordinary business APIs never select a tenant
from a request header, query parameter, or mutation body.

New sync cursors bind `tenant:<tenantUUID>:<spaceUUID>:<generation>:<position>`.
Only the migrated Telomoyo tenant accepts old numeric Production cursors and
`sandbox:<generation>:<position>` cursors. Already-signed pre-migration operations
are assessed against their persisted origin session; do not reserialize, re-sign,
or reattribute them because the uploading app was upgraded.

Tenant switching is online-only and waits for active sync and the complete
physical print attempt. Normal switching drains the current outbox first. Once
suspension, inactive membership, or terminal revocation is discovered, affected
business screens and printing are locked. Pending entries remain quarantined by
tenant, space, origin membership/session, and enrollment; selecting another
authorized tenant must not release them. Restoration requires matching authority
and online origin revalidation (`POST /sync/revalidate`). Never clear app data or
rotate another tenant's signing key to recover one revoked enrollment.

While a transaction has quarantined entries, incoming sync snapshots do not
overwrite its local transaction/revision evidence. Corrections, payment changes,
printing, and recovery cannot bypass that protection. A write already signing
when access is revoked inherits quarantine when it enters the outbox. Explicit
successful origin revalidation resets the scoped pull cursor so authoritative
history skipped during quarantine is fetched again; signed bytes are unchanged.

## Merchant configuration and history

Each tenant's business profile and QRIS payloads are versioned centrally. Only a
Production-mode tenant superadmin can update them. QRIS caches are encrypted and
scoped; historical payments remain bound to their exact payload hash. Explicit
Telomoyo-superadmin confirmation is required to import an old device payload.
Several old payloads may be imported as historical versions; only one is active.
An unavailable historical payload must not silently display a newer merchant QR.

New transaction receipts snapshot business name/address/phone and profile revision.
Corrections and reprints preserve that identity, with a small Telomoyo POS credit.
Printing requires successful payment and freezes the document throughout the
physical attempt. Sandbox output retains top/bottom test watermarks, TEST IDs,
simulated totals, and actual QRIS charge. Export filenames and contents identify
both the business and test mode where applicable.

## Monitoring and backup boundary

New Relic transactions, correlated logs, and noticed errors carry `auth.context`.
Authenticated tenant requests additionally carry `tenant.id`, `data.mode`,
`data.space_id`, and Sandbox generation. Public/account/platform requests do not
pretend to have a Production data space. Scope business error/revenue views by
tenant and mode; do not filter infrastructure availability/migration alerts by
business scope. Platform operations have a separate durable control-plane audit.

```sql
SELECT count(*) FROM TransactionError
WHERE `auth.context` = 'tenant' AND `data.mode` = 'production'
FACET `tenant.id`
```

Never include tokens, passwords/hashes, invitation codes, signing keys, raw QRIS,
or database connection strings in logs. Observe cleanup failure separately from
Production service readiness. Reset/purge audit must outlive deleted Sandbox
business evidence and identify the affected tenant/generation.

Infrastructure backups remain **operator-only and contain the shared database**,
including all tenants and both modes. Do not deliver a raw snapshot/`pg_dump` to a
tenant or treat a report export as a full recovery backup. Apply encryption,
access control, and retention to snapshots and restore environments. Tenant
deletion and per-tenant full-backup delivery are not part of this release.

## Release acceptance checklist

These are release gates, not a statement that hardware acceptance has completed.

- Migrate populated data and compare IDs, totals, revisions, legacy sessions,
  encrypted files/keys, and exact signed outbox bytes before/after the upgrade.
- With two tenants and a shared account with different roles, prove isolation of
  packages, QRIS, profile, history, dashboards, export, audit, idempotency, cursors,
  terminals, user projections, clone sources, payments, and print attempts.
- Check cross-tenant identifiers return 404 and missing/unauthorized context fails
  closed. Account/platform contexts cannot use business APIs; platform authority
  alone cannot select a business. Legacy global-user mutations require upgrade.
- Test invitation expiry/revocation/reissue and concurrent acceptance/registration,
  atomic owner activation, per-tenant last-superadmin/self-demotion protection,
  global password recovery, and inactive historical members.
- Race suspension, membership changes, terminal revocation, and Sandbox reset
  against mutations. Verify lock order account → tenant → membership → data space
  → resource; restored access does not revive revoked sessions.
- Test switch with pending work, offline/interrupted setup, app restart/logout,
  background/network-recovery sync, and active physical printing. Verify exact
  origin quarantine and another account/tenant cannot release or replay it.
- Decode QRIS for two real merchant payloads in both modes, verify CRC, exact
  Rp1.000 Sandbox amount, full Production amount, and historical hash binding.
- Verify profile snapshot reprints, tenant export identity, Sandbox watermarks,
  independent reset/30-day purge, durable cleanup audit, and scoped New Relic data.
- Run backend unit/integration tests against disposable PostgreSQL schemas,
  mobile tests, generated-contract checks, and an Android build. On target MPOS
  hardware exercise shared-device tenant switching during integrated/Bluetooth
  printing and recovery from disconnect/uncertain print results.

## Local verification record — 11 September 2026

Verified against the current working tree, without deploying or enabling
provisioning:

- Backend `go test ./... -count=1` with PostgreSQL enabled passed, including
  disposable-schema tenant isolation, invitation/registration rollback,
  concurrent owner protection, Sandbox lifecycle, migration, and operator tests.
- Mobile Jest: **40 suites, 246 tests passed**. Coverage includes tenant/mode
  encrypted-storage naming, legacy key/session preservation, context exchanges,
  pending-outbox/print barriers, failed session/database setup, exact-origin
  quarantine, and historical merchant/receipt binding.
- Real in-memory SQLite tests ran the actual mobile migrations and repository
  SQL, proving empty new-tenant catalogs and preservation of quarantined
  transaction/revision/signature bytes during another actor's pull/recovery.
  These tests use the built-in `node:sqlite` module; the verified runtime was
  Node 24.19.0 (no mobile runtime dependency was added).
- Mobile TypeScript and ESLint checks passed. OpenAPI validation, generated
  TypeScript/Go client checks, sqlc consistency, and Compose validation passed.
- ARM64 Android `assembleDebug` succeeded for
  `com.fahmialfareza.sewamotorpos` version 0.2.0/code 2. Expo Android production
  JavaScript export also succeeded. A debug APK still uses the development
  client; this is not a signed production-release build.

Not yet performed: installed-device upgrade with real pending queues, physical
MPOS/Bluetooth switching and printing, real merchant scan/payment reconciliation,
production-restore rehearsal, live New Relic dashboard/alert validation, or the
production rollout. Complete these release gates before enabling provisioning.
Tests used synthetic merchant fixtures and did not send payments. Production
schema/data, platform-admin assignment, and feature flags were not changed.
