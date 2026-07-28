import { create } from "zustand";

import { useAuthStore } from "@/auth/auth-store";
import { countPendingOutbox, getSyncMetadata } from "@/db/repositories";
import { readSession, readTerminalIdentity } from "@/security/secure-store";

import { runSync, type SyncSummary } from "./engine";

export interface SyncStore {
  online: boolean;
  syncing: boolean;
  pendingCount: number;
  lastSyncedAt: string | null;
  lastError: string | null;
  lastSummary: SyncSummary | null;
  setOnline: (online: boolean) => void;
  refresh: () => Promise<void>;
  syncNow: () => Promise<SyncSummary | null>;
}

let activeStoreSync: Promise<SyncSummary | null> | null = null;

export const useSyncStore = create<SyncStore>((set, get) => ({
  online: true,
  syncing: false,
  pendingCount: 0,
  lastSyncedAt: null,
  lastError: null,
  lastSummary: null,

  setOnline: (online) => set({ online }),

  refresh: async () => {
    const [pendingCount, metadata] = await Promise.all([
      countPendingOutbox(),
      getSyncMetadata(),
    ]);
    set({
      pendingCount,
      lastSyncedAt: metadata.lastSyncedAt,
      lastError: metadata.lastError,
    });
  },

  syncNow: () => {
    const session = useAuthStore.getState().session;
    if (!session || !get().online) return Promise.resolve(null);
    if (activeStoreSync) return activeStoreSync;

    set({ syncing: true });
    activeStoreSync = runSync(session)
      .then(async (summary) => {
        const [storedSession, terminal] = await Promise.all([
          readSession(),
          readTerminalIdentity(),
        ]);
        if (storedSession?.sessionId === session.sessionId) {
          useAuthStore.setState({
            session: storedSession,
            terminalEnrolled: Boolean(terminal?.enrolledAt),
          });
        }
        set({ lastSummary: summary, lastError: null });
        return summary;
      })
      .catch((error: unknown) => {
        set({
          lastError:
            error instanceof Error ? error.message : "Sinkronisasi gagal.",
        });
        throw error;
      })
      .finally(async () => {
        set({ syncing: false });
        try {
          await get().refresh();
        } finally {
          activeStoreSync = null;
        }
      });
    return activeStoreSync;
  },
}));
