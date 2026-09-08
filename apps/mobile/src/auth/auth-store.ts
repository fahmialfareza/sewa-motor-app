import NetInfo from "@react-native-community/netinfo";
import Constants from "expo-constants";
import { create } from "zustand";

import { apiRequest } from "@/api/client";
import type { LoginResponse } from "@/api/contracts";
import { sessionFromLoginResponse } from "@/auth/session";
import { prepareDatabaseForSession } from "@/db/client";
import {
  countPendingOutbox,
  recoverInterruptedPrintAttempts,
} from "@/db/repositories";
import {
  PRODUCTION_DATA_SPACE_ID,
  type DataMode,
  type Role,
  type Session,
} from "@/domain/types";
import { setModeFromSession } from "@/mode/mode-store";
import {
  beginModeTransition,
  beginModeSafeLocalAccess,
  MODE_TRANSITION_BUSY_MESSAGE,
  type ModeTransitionLease,
} from "@/mode/mutation-barrier";
import {
  isSandboxGenerationRetired,
  recoverRetiredSandboxGeneration,
  retireSandboxSession,
  SANDBOX_RECOVERY_ERROR,
  SANDBOX_RETIRED_MESSAGE,
} from "@/mode/recovery";
import {
  clearAuthNotice,
  clearSession,
  readAuthNotice,
  readSession,
  writeSession,
} from "@/security/secure-store";
import {
  getOrCreateTerminalIdentity,
  getTerminalPublicKeyBase64,
  markTerminalEnrolled,
  markTerminalRevoked,
} from "@/security/terminal-identity";
import { runSync } from "@/sync/engine";
import {
  hydrateSyncStateForSession,
  resetSyncStateForSession,
} from "@/sync/state-handoff";

export interface AuthStore {
  session: Session | null;
  booting: boolean;
  bootError: string | null;
  notice: string | null;
  demoEnabled: boolean;
  terminalEnrolled: boolean;
  switchingMode: boolean;
  dismissNotice: () => Promise<void>;
  hydrate: () => Promise<void>;
  login: (username: string, password: string) => Promise<void>;
  demoLogin: (role: Role) => Promise<void>;
  switchMode: (mode: DataMode) => Promise<void>;
  changePassword: (
    currentPassword: string,
    newPassword: string,
  ) => Promise<void>;
  enrollTerminal: (label: string) => Promise<void>;
  logout: () => Promise<void>;
}

const demoEnabled =
  __DEV__ &&
  (Constants.expoConfig?.extra?.enableDemoLogin as boolean | undefined) ===
    true;

let hydration: Promise<void> | null = null;

const MODE_DATABASE_ERROR =
  "Penyimpanan terenkripsi untuk mode baru belum dapat dibuka. Tutup dan buka kembali aplikasi untuk mencoba lagi.";

const MODE_SESSION_ERROR =
  "Sesi mode baru belum dapat diamankan pada perangkat. Masuk kembali sebelum melanjutkan.";

const LOGOUT_STORAGE_ERROR =
  "Logout berhasil di server, tetapi sesi lokal belum dapat dibersihkan. Tutup dan buka kembali aplikasi sebelum melanjutkan.";

function exposeSession(
  session: Session,
  set: (state: Partial<AuthStore>) => void,
  bootError: string | null = null,
) {
  setModeFromSession(session);
  set({ session, bootError });
}

async function persistSession(
  session: Session,
  set: (state: Partial<AuthStore>) => void,
) {
  // Persist first, prepare the explicitly scoped store, then expose the
  // session to providers. If preparation fails, expose only behind bootError
  // so screens cannot render data from a previously active scope.
  try {
    await writeSession(session);
  } catch (error) {
    resetSyncStateForSession(session);
    exposeSession(session, set, MODE_SESSION_ERROR);
    throw error;
  }
  try {
    await prepareDatabaseForSession(session);
    await hydrateSyncStateForSession(session);
  } catch (error) {
    // A successfully issued session must replace the old scope even when its
    // local database cannot be opened. Blocking the protected layout prevents
    // data from the previous mode from rendering under the new credentials.
    resetSyncStateForSession(session);
    exposeSession(session, set, MODE_DATABASE_ERROR);
    throw error;
  }
  exposeSession(session, set);
}

async function recoverRetiredSandboxSession(
  session: Session,
  targetMode: DataMode,
  set: (state: Partial<AuthStore>) => void,
  transitionLease: ModeTransitionLease,
): Promise<void> {
  // Remove the authenticated tree before recovery changes the active database,
  // so no screen can read a replacement scope before its first pull completes.
  set({
    session: null,
    terminalEnrolled: false,
    notice: SANDBOX_RETIRED_MESSAGE,
    bootError: null,
  });
  resetSyncStateForSession(null);
  try {
    const recovered = await recoverRetiredSandboxGeneration(
      session,
      targetMode,
      async (result) => {
        await hydrateSyncStateForSession(result.session, result.summary);
      },
      { transitionLease },
    );
    exposeSession(recovered.session, set);
    set({
      terminalEnrolled: recovered.terminalEnrolled,
      notice: recovered.notice,
    });
  } catch (error) {
    set({
      session: null,
      terminalEnrolled: false,
      notice: SANDBOX_RECOVERY_ERROR,
      bootError: null,
    });
    throw error;
  }
}

function modeChangedButSetupIncomplete(
  error: unknown,
  state: { localScopeReady: boolean; durable: boolean },
): Error {
  const message = !state.localScopeReady
    ? state.durable
      ? "Mode operasi sudah berhasil diganti di server dan sesi baru sudah disimpan, tetapi penyimpanan terenkripsi belum dapat dibuka. Tutup lalu buka kembali aplikasi untuk mencoba lagi."
      : "Mode operasi sudah berhasil diganti di server, tetapi sesi dan penyimpanan terenkripsi baru belum dapat diamankan di perangkat. Jangan tutup aplikasi dan hubungi dukungan."
    : state.durable
      ? "Mode operasi sudah berhasil diganti, tetapi data awal belum selesai disinkronkan. Sesi baru tetap tersimpan; periksa koneksi lalu coba Sinkronkan sekarang."
      : "Mode operasi di server sudah berhasil diganti, tetapi sesi baru belum dapat disimpan di perangkat. Database mode baru sudah aman; jangan tutup aplikasi dan coba ganti mode kembali setelah penyimpanan perangkat tersedia.";
  const wrapped = new Error(message);
  Object.defineProperty(wrapped, "cause", {
    configurable: true,
    value: error,
  });
  return wrapped;
}

export const useAuthStore = create<AuthStore>((set, get) => ({
  session: null,
  booting: true,
  bootError: null,
  notice: null,
  demoEnabled,
  terminalEnrolled: false,
  switchingMode: false,

  dismissNotice: async () => {
    await clearAuthNotice();
    set({ notice: null });
  },

  hydrate: () => {
    hydration ??= beginModeSafeLocalAccess()
      .then(async (releaseLocalAccess) => {
        try {
          const [storedSession, terminal, notice] = await Promise.all([
            readSession(),
            getOrCreateTerminalIdentity(),
            readAuthNotice(),
          ]);
          setModeFromSession(storedSession);
          resetSyncStateForSession(storedSession);
          if (storedSession) {
            await prepareDatabaseForSession(storedSession);
          } else {
            await prepareDatabaseForSession({
              dataMode: "production",
              sandboxGeneration: null,
            });
          }
          if (storedSession) {
            await recoverInterruptedPrintAttempts(storedSession).catch(
              () => undefined,
            );
          }
          await hydrateSyncStateForSession(storedSession);
          // A background reset may have requested the transition while this
          // startup read held the local-access lease. It now owns exposure of
          // the replacement session, so never put the retired one back.
          if (!get().switchingMode) {
            set({
              session: storedSession,
              terminalEnrolled: Boolean(terminal.enrolledAt),
              notice,
              bootError: null,
            });
          }
        } finally {
          releaseLocalAccess();
        }
      })
      .catch((error: unknown) => {
        set({
          bootError:
            error instanceof Error
              ? error.message
              : "Penyimpanan lokal tidak dapat disiapkan.",
        });
        throw error;
      })
      .finally(() => set({ booting: false }));
    return hydration;
  },

  login: async (username, password) => {
    const terminal = await getOrCreateTerminalIdentity();
    const result = await apiRequest<LoginResponse>("/auth/login", {
      method: "POST",
      body: {
        username: username.trim(),
        password,
        installationId: terminal.installationId,
      },
    });
    await persistSession(sessionFromLoginResponse(result), set);
    await clearAuthNotice();
    set({ notice: null });
    if (result.terminal) {
      await markTerminalEnrolled(result.terminal.id);
      set({ terminalEnrolled: true });
      return;
    }
    if (terminal.serverTerminalId) {
      await markTerminalRevoked(terminal.serverTerminalId);
    }
    set({ terminalEnrolled: false });
  },

  demoLogin: async (role) => {
    if (!get().demoEnabled) {
      throw new Error("Login demo hanya tersedia pada development build.");
    }
    await persistSession(
      {
        token: `dev-only-${role}`,
        sessionId: `DEV-SESSION-${role}`,
        user: {
          id: `DEV-${role.toUpperCase()}`,
          fullName: role === "superadmin" ? "Penyok" : "Putu",
          username: role,
          role,
          active: true,
          mustChangePassword: false,
        },
        establishedAt: new Date().toISOString(),
        dataMode: "production",
        dataSpaceId: PRODUCTION_DATA_SPACE_ID,
        sandboxGeneration: null,
      },
      set,
    );
    const terminal = await getOrCreateTerminalIdentity();
    if (!terminal.enrolledAt) {
      await markTerminalEnrolled("00000000-0000-4000-8000-000000000099");
    }
    set({ terminalEnrolled: true });
  },

  switchMode: async (mode) => {
    const session = get().session;
    if (!session) throw new Error("Sesi tidak tersedia.");
    if (session.dataMode === mode) return;
    if (session.token.startsWith("dev-only-")) {
      throw new Error("Mode Uji server tidak tersedia pada sesi demo lokal.");
    }
    if (get().switchingMode) {
      throw new Error(MODE_TRANSITION_BUSY_MESSAGE);
    }

    set({ switchingMode: true });
    let nextSession: Session | null = null;
    let replacementDatabaseReady = false;
    let replacementMetadataReady = false;
    let replacementPersisted = false;
    let transitionLease: ModeTransitionLease | null = null;
    try {
      transitionLease = await beginModeTransition();
      const network = await NetInfo.fetch();
      if (!network.isConnected || network.isInternetReachable === false) {
        throw new Error(
          "Pergantian mode memerlukan internet agar data saat ini tetap aman.",
        );
      }

      await runSync(session);
      if ((await countPendingOutbox(session.dataMode)) > 0) {
        throw new Error(
          "Masih ada perubahan, konflik, atau operasi ditolak yang belum diselesaikan di Pusat Sinkron.",
        );
      }

      const result = await apiRequest<LoginResponse>("/auth/switch-mode", {
        method: "POST",
        token: session.token,
        body: { mode },
      });
      nextSession = sessionFromLoginResponse(result);
      try {
        await writeSession(nextSession);
        replacementPersisted = true;
      } catch (firstWriteError) {
        // The server has already revoked the previous session. Remove its
        // durable copy if possible, prepare the generation-scoped database
        // before exposing the replacement, and retry secure persistence once.
        await clearSession().catch(() => undefined);
        await prepareDatabaseForSession(nextSession);
        replacementDatabaseReady = true;
        try {
          await writeSession(nextSession);
          replacementPersisted = true;
        } catch {
          throw firstWriteError;
        }
      }
      if (!replacementDatabaseReady) {
        await prepareDatabaseForSession(nextSession);
        replacementDatabaseReady = true;
      }
      const summary = await runSync(nextSession);
      await hydrateSyncStateForSession(nextSession, summary);
      replacementMetadataReady = true;
      exposeSession(nextSession, set);
    } catch (error) {
      const affectedSession = nextSession ?? session;
      if (
        affectedSession.dataMode === "sandbox" &&
        isSandboxGenerationRetired(error)
      ) {
        if (!transitionLease) throw error;
        await recoverRetiredSandboxSession(
          affectedSession,
          mode,
          set,
          transitionLease,
        );
        return;
      }
      if (nextSession) {
        let setupError = error;
        if (replacementDatabaseReady && !replacementMetadataReady) {
          try {
            await hydrateSyncStateForSession(nextSession);
            replacementMetadataReady = true;
          } catch (metadataError) {
            setupError = metadataError;
          }
        }
        const localScopeReady =
          replacementDatabaseReady && replacementMetadataReady;
        if (!localScopeReady) {
          resetSyncStateForSession(nextSession);
        }
        exposeSession(
          nextSession,
          set,
          !replacementPersisted
            ? MODE_SESSION_ERROR
            : localScopeReady
              ? null
              : MODE_DATABASE_ERROR,
        );
        throw modeChangedButSetupIncomplete(setupError, {
          localScopeReady,
          durable: replacementPersisted,
        });
      }
      throw error;
    } finally {
      transitionLease?.release();
      set({ switchingMode: false });
    }
  },

  changePassword: async (currentPassword, newPassword) => {
    const session = get().session;
    if (!session) throw new Error("Sesi tidak tersedia.");
    if (session.dataMode === "sandbox") {
      throw new Error(
        "Kata sandi tidak dapat diubah dari Mode Uji. Kembali ke Mode Produksi terlebih dahulu.",
      );
    }
    if (!session.token.startsWith("dev-only-")) {
      await apiRequest("/profile/password", {
        method: "POST",
        token: session.token,
        body: { currentPassword, newPassword },
      });
    }
    await persistSession(
      {
        ...session,
        user: { ...session.user, mustChangePassword: false },
      },
      set,
    );
  },

  enrollTerminal: async (label) => {
    const session = get().session;
    if (!session) throw new Error("Sesi tidak tersedia.");
    if (session.dataMode === "sandbox") {
      throw new Error(
        "Terminal tidak dapat didaftarkan dari Mode Uji. Kembali ke Mode Produksi terlebih dahulu.",
      );
    }
    const terminal = await getOrCreateTerminalIdentity();
    let serverTerminalId = "00000000-0000-4000-8000-000000000099";
    if (!session.token.startsWith("dev-only-")) {
      const publicKey = await getTerminalPublicKeyBase64();
      const result = await apiRequest<{ id: string }>("/terminals/enroll", {
        method: "POST",
        token: session.token,
        body: {
          installationId: terminal.installationId,
          name: label.trim(),
          publicKey,
          algorithm: "Ed25519",
        },
      });
      serverTerminalId = result.id;
    }
    await markTerminalEnrolled(serverTerminalId);
    set({ terminalEnrolled: true });
  },

  logout: async () => {
    const session = get().session;
    if (!session) return;
    if (get().switchingMode) {
      throw new Error(MODE_TRANSITION_BUSY_MESSAGE);
    }

    set({ switchingMode: true });
    let transitionLease: ModeTransitionLease | null = null;
    try {
      transitionLease = await beginModeTransition();
      const network = await NetInfo.fetch();
      if (!network.isConnected || network.isInternetReachable === false) {
        throw new Error("Logout memerlukan koneksi internet.");
      }
      // Always join any foreground pull before clearing SecureStore. An active
      // sync may refresh the user snapshot even when the outbox is empty; if it
      // finished after logout it could otherwise restore the revoked session.
      await runSync(session);
      if ((await countPendingOutbox(session.dataMode)) > 0) {
        throw new Error(
          "Masih ada perubahan yang belum tersinkron. Selesaikan sebelum logout.",
        );
      }
      if (!session.token.startsWith("dev-only-")) {
        await apiRequest("/auth/logout", {
          method: "POST",
          token: session.token,
        });
      }
      try {
        await clearSession();
      } catch (error) {
        setModeFromSession(null);
        resetSyncStateForSession(null);
        set({
          session: null,
          terminalEnrolled: false,
          bootError: LOGOUT_STORAGE_ERROR,
        });
        throw error;
      }
      setModeFromSession(null);
      resetSyncStateForSession(null);
      set({ session: null, terminalEnrolled: false, bootError: null });
    } catch (error) {
      if (session.dataMode === "sandbox" && isSandboxGenerationRetired(error)) {
        // Logout never rotates into another authenticated session. Clear only
        // the retired Sandbox scope and retain the normal Production default.
        set({ session: null, terminalEnrolled: false, bootError: null });
        resetSyncStateForSession(null);
        await retireSandboxSession(session);
        return;
      }
      throw error;
    } finally {
      transitionLease?.release();
      set({ switchingMode: false });
    }
  },
}));
