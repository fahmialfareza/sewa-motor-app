import NetInfo from "@react-native-community/netinfo";
import Constants from "expo-constants";
import { create } from "zustand";

import { apiRequest } from "@/api/client";
import type { LoginResponse } from "@/api/contracts";
import {
  countPendingOutbox,
  recoverInterruptedPrintAttempts,
} from "@/db/repositories";
import type { Role, Session } from "@/domain/types";
import {
  clearSession,
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

export interface AuthStore {
  session: Session | null;
  booting: boolean;
  demoEnabled: boolean;
  terminalEnrolled: boolean;
  hydrate: () => Promise<void>;
  login: (username: string, password: string) => Promise<void>;
  demoLogin: (role: Role) => Promise<void>;
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

async function persistSession(
  session: Session,
  set: (state: Partial<AuthStore>) => void,
) {
  await writeSession(session);
  set({ session });
}

export const useAuthStore = create<AuthStore>((set, get) => ({
  session: null,
  booting: true,
  demoEnabled,
  terminalEnrolled: false,

  hydrate: () => {
    hydration ??= Promise.all([readSession(), getOrCreateTerminalIdentity()])
      .then(async ([storedSession, terminal]) => {
        if (storedSession) {
          await recoverInterruptedPrintAttempts(storedSession).catch(
            () => undefined,
          );
        }
        set({
          session: storedSession,
          terminalEnrolled: Boolean(terminal.enrolledAt),
        });
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
    await persistSession(
      {
        token: result.sessionToken,
        sessionId: result.sessionId,
        user: result.user,
        establishedAt: new Date().toISOString(),
      },
      set,
    );
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
          fullName: role === "superadmin" ? "Budi Santoso" : "Andi Wijaya",
          username: role,
          role,
          active: true,
          mustChangePassword: false,
        },
        establishedAt: new Date().toISOString(),
      },
      set,
    );
    const terminal = await getOrCreateTerminalIdentity();
    if (!terminal.enrolledAt) {
      await markTerminalEnrolled("00000000-0000-4000-8000-000000000099");
    }
    set({ terminalEnrolled: true });
  },

  changePassword: async (currentPassword, newPassword) => {
    const session = get().session;
    if (!session) throw new Error("Sesi tidak tersedia.");
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
    const network = await NetInfo.fetch();
    if (!network.isConnected || network.isInternetReachable === false) {
      throw new Error("Logout memerlukan koneksi internet.");
    }
    if ((await countPendingOutbox()) > 0) {
      await runSync(session);
      if ((await countPendingOutbox()) > 0) {
        throw new Error(
          "Masih ada perubahan yang belum tersinkron. Selesaikan sebelum logout.",
        );
      }
    }
    if (!session.token.startsWith("dev-only-")) {
      await apiRequest("/auth/logout", {
        method: "POST",
        token: session.token,
      });
    }
    await clearSession();
    set({ session: null, terminalEnrolled: false });
  },
}));
