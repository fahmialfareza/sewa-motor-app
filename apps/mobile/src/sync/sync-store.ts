import { create } from "zustand";

import { useAuthStore } from "@/auth/auth-store";
import { countPendingOutbox, getSyncMetadata } from "@/db/repositories";
import type { Session } from "@/domain/types";
import {
  activeRetiredSandboxRecovery,
  isSandboxGenerationRetired,
  recoverRetiredSandboxGeneration,
  SANDBOX_RECOVERY_ERROR,
  SANDBOX_RETIRED_MESSAGE,
} from "@/mode/recovery";
import { beginModeSafeLocalAccess } from "@/mode/mutation-barrier";
import { readSession, readTerminalIdentity } from "@/security/secure-store";
import { toUserFacingErrorMessage } from "@/utils/errors";

import { runSync, type SyncSummary } from "./engine";
import { registerSyncStateHandoff } from "./state-handoff";

export interface SyncStore {
  dataSpaceId: string | null;
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

function sameSession(left: Session | null, right: Session): boolean {
  return left?.sessionId === right.sessionId && left.token === right.token;
}

function capturedSessionIsActive(session: Session): boolean {
  const auth = useAuthStore.getState();
  return !auth.switchingMode && sameSession(auth.session, session);
}

function emptyScopedState(session: Session | null) {
  return {
    dataSpaceId: session?.dataSpaceId ?? null,
    syncing: false,
    pendingCount: 0,
    lastSyncedAt: null,
    lastError: null,
    lastSummary: null,
  };
}

export function resetSyncStoreForSession(session: Session | null): void {
  useSyncStore.setState(emptyScopedState(session));
}

export async function hydrateSyncStoreForSession(
  session: Session | null,
  summary: SyncSummary | null = null,
): Promise<void> {
  resetSyncStoreForSession(session);
  if (!session || (session.contextKind && session.contextKind !== "tenant"))
    return;

  const requestedDataSpaceId = session.dataSpaceId;
  const [pendingCount, metadata] = await Promise.all([
    countPendingOutbox(session),
    getSyncMetadata(session),
  ]);
  if (useSyncStore.getState().dataSpaceId !== requestedDataSpaceId) return;

  useSyncStore.setState({
    pendingCount,
    lastSyncedAt: metadata.lastSyncedAt,
    lastError: metadata.lastError
      ? toUserFacingErrorMessage(
          metadata.lastError,
          "Sinkronisasi belum berhasil. Coba lagi.",
        )
      : null,
    lastSummary: summary,
  });
}

export const useSyncStore = create<SyncStore>((set, get) => ({
  dataSpaceId: null,
  online: true,
  syncing: false,
  pendingCount: 0,
  lastSyncedAt: null,
  lastError: null,
  lastSummary: null,

  setOnline: (online) => set({ online }),

  refresh: async () => {
    const auth = useAuthStore.getState();
    if (auth.switchingMode) return;
    const session = auth.session;
    if (!session || (session.contextKind && session.contextKind !== "tenant")) {
      resetSyncStoreForSession(null);
      return;
    }
    if (get().dataSpaceId !== session.dataSpaceId) {
      resetSyncStoreForSession(session);
    }
    const requestedDataSpaceId = session.dataSpaceId;
    const [pendingCount, metadata] = await Promise.all([
      countPendingOutbox(session),
      getSyncMetadata(session),
    ]);
    const currentAuth = useAuthStore.getState();
    if (
      currentAuth.switchingMode ||
      currentAuth.session?.dataSpaceId !== requestedDataSpaceId ||
      get().dataSpaceId !== requestedDataSpaceId
    ) {
      return;
    }
    set({
      pendingCount,
      lastSyncedAt: metadata.lastSyncedAt,
      lastError: metadata.lastError
        ? toUserFacingErrorMessage(
            metadata.lastError,
            "Sinkronisasi belum berhasil. Coba lagi.",
          )
        : null,
    });
  },

  syncNow: () => {
    const auth = useAuthStore.getState();
    const session = auth.session;
    if (
      !session ||
      auth.scopeLocked ||
      (session.contextKind && session.contextKind !== "tenant") ||
      auth.switchingMode ||
      !get().online
    ) {
      return Promise.resolve(null);
    }
    if (get().dataSpaceId !== session.dataSpaceId) {
      resetSyncStoreForSession(session);
    }
    if (activeStoreSync) return activeStoreSync;

    set({ syncing: true });
    let skipRefresh = false;
    let releaseLocalAccess: (() => void) | null = null;
    activeStoreSync = beginModeSafeLocalAccess()
      .then((release) => {
        releaseLocalAccess = release;
        // The session may have changed while this sync waited behind a mode
        // transition. Never run with credentials captured from the old scope.
        if (!capturedSessionIsActive(session)) {
          skipRefresh = true;
          return null;
        }
        return runSync(session);
      })
      .then(async (summary) => {
        if (!summary) return null;
        // A transition can request exclusivity while runSync is in flight. Its
        // barrier waits for this lease, but the UI session must not be refreshed
        // once that request exists.
        if (!capturedSessionIsActive(session)) return summary;
        const [storedSession, terminal] = await Promise.all([
          readSession(),
          readTerminalIdentity(session.tenantId ?? undefined),
        ]);
        const currentAuth = useAuthStore.getState();
        if (
          !currentAuth.switchingMode &&
          currentAuth.session?.sessionId === session.sessionId &&
          currentAuth.session.token === session.token &&
          storedSession?.sessionId === session.sessionId &&
          storedSession.token === session.token
        ) {
          useAuthStore.setState({
            session: storedSession,
            terminalEnrolled: Boolean(terminal?.enrolledAt),
          });
        }
        if (get().dataSpaceId === session.dataSpaceId) {
          set({ lastSummary: summary, lastError: null });
        }
        return summary;
      })
      .catch(async (error: unknown) => {
        if (isSandboxGenerationRetired(error)) {
          skipRefresh = true;
          // Recovery needs the exclusive transition lease. Release this sync's
          // shared local-access lease before either joining or starting it.
          releaseLocalAccess?.();
          releaseLocalAccess = null;
          const auth = useAuthStore.getState();
          if (!sameSession(auth.session, session)) throw error;
          // A user-initiated switch already owns the barrier and decides the
          // destination mode. Join it when recovery has started instead of
          // launching a competing Sandbox-target rotation.
          if (auth.switchingMode) {
            const coordinated = activeRetiredSandboxRecovery(session);
            if (!coordinated) throw error;
            return (await coordinated).summary;
          }

          useAuthStore.setState({ switchingMode: true });
          resetSyncStoreForSession(null);
          useAuthStore.setState({
            session: null,
            terminalEnrolled: false,
            notice: SANDBOX_RETIRED_MESSAGE,
            bootError: null,
          });
          try {
            const recovered = await recoverRetiredSandboxGeneration(
              session,
              "sandbox",
              async (result) => {
                await hydrateSyncStoreForSession(
                  result.session,
                  result.summary,
                );
              },
            );
            useAuthStore.setState({
              session: recovered.session,
              terminalEnrolled: recovered.terminalEnrolled,
              notice: recovered.notice,
              bootError: null,
            });
            return recovered.summary;
          } catch (recoveryError) {
            useAuthStore.setState({
              session: null,
              terminalEnrolled: false,
              notice: SANDBOX_RECOVERY_ERROR,
              bootError: null,
            });
            throw recoveryError;
          } finally {
            useAuthStore.setState({ switchingMode: false });
          }
        }
        if (
          capturedSessionIsActive(session) &&
          get().dataSpaceId === session.dataSpaceId
        ) {
          set({
            lastError: toUserFacingErrorMessage(
              error,
              "Sinkronisasi belum berhasil. Coba lagi.",
            ),
          });
        }
        throw error;
      })
      .finally(async () => {
        if (get().dataSpaceId === session.dataSpaceId) {
          set({ syncing: false });
        }
        try {
          if (!skipRefresh) await get().refresh();
        } finally {
          releaseLocalAccess?.();
          activeStoreSync = null;
        }
      });
    return activeStoreSync;
  },
}));

registerSyncStateHandoff({
  reset: resetSyncStoreForSession,
  hydrate: hydrateSyncStoreForSession,
});
