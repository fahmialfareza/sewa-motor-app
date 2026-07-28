# Sewa Motor POS Mobile

Expo SDK 57 / React Native 0.86 Android client for local-first operation on
phones and MPOS terminals.

## State and persistence

- Zustand owns live authentication and synchronization state in
  `src/auth/auth-store.ts` and `src/sync/sync-store.ts`.
- SecureStore remains the encrypted persistence boundary for session tokens and
  the terminal Ed25519 private key. Secrets are not persisted by a general
  Zustand storage adapter.
- Encrypted SQLite is the UI read model and durable offline queue. Signed outbox
  operations are sent FIFO and remote changes are pulled by cursor.
- Provider components only attach startup, network, foreground, and background
  lifecycle events. Screens consume selector-backed hooks to avoid broad context
  rerenders.

## Development

```sh
pnpm --filter @sewa-motor/mobile check-types
pnpm --filter @sewa-motor/mobile lint
pnpm --filter @sewa-motor/mobile test
pnpm --filter @sewa-motor/mobile start
```

Expo Go cannot load SQLCipher or the local printer module. Generate only the
Android native project and run a development client:

```sh
ANDROID_APPLICATION_ID=com.yourcompany.sewamotor \
  pnpm --filter @sewa-motor/mobile native:prebuild
pnpm --filter @sewa-motor/mobile android
```

`pnpm --filter @sewa-motor/mobile build` performs a Metro Android export. A
signed APK/AAB still requires the final Android application ID, EAS project
ownership, keystore/Play configuration, and environment-specific API URL.

## Printer boundary

The local Expo module exposes integrated and Bluetooth adapter boundaries plus a
simulator. The vendor AAR/JAR, supported paper width, encoding, timeout
semantics, and device identifiers must be supplied for the selected MPOS model
before physical acceptance. Every attempted print is stored with
pending/success/failed/unknown state so an uncertain hardware result is never
silently treated as success.
