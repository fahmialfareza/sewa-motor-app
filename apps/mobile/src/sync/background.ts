import * as BackgroundTask from "expo-background-task";
import * as TaskManager from "expo-task-manager";

import { useAuthStore } from "@/auth/auth-store";
import { prepareDatabaseForSession } from "@/db/client";
import type { Session } from "@/domain/types";
import { setModeFromSession } from "@/mode/mode-store";
import { beginModeSafeLocalAccess } from "@/mode/mutation-barrier";
import {
  activeRetiredSandboxRecovery,
  isSandboxGenerationRetired,
  recoverRetiredSandboxGeneration,
  SANDBOX_RECOVERY_ERROR,
  SANDBOX_RETIRED_MESSAGE,
} from "@/mode/recovery";
import { readSession } from "@/security/secure-store";
import { blockedScopeReason } from "@/tenant/quarantine";

import { runSync } from "./engine";
import {
  hydrateSyncStateForSession,
  resetSyncStateForSession,
} from "./state-handoff";

export const BACKGROUND_SYNC_TASK = "sewa-motor-background-sync";
export const BACKGROUND_SYNC_MINIMUM_INTERVAL_MINUTES = 15;

function sameSession(left: Session | null, right: Session): boolean {
  return left?.sessionId === right.sessionId && left.token === right.token;
}

if (!TaskManager.isTaskDefined(BACKGROUND_SYNC_TASK)) {
  TaskManager.defineTask(BACKGROUND_SYNC_TASK, async () => {
    let session: Session | null = null;
    let releaseLocalAccess: (() => void) | null = null;
    try {
      // A background task must not read an old scope or write an old user
      // snapshot to SecureStore after a foreground mode rotation. This lease
      // either runs the entire sync before the transition or waits and reads
      // the replacement session afterward.
      releaseLocalAccess = await beginModeSafeLocalAccess();
      session = await readSession();
      if (!session) return BackgroundTask.BackgroundTaskResult.Success;
      if (
        (session.contextKind && session.contextKind !== "tenant") ||
        useAuthStore.getState().scopeLocked
      )
        return BackgroundTask.BackgroundTaskResult.Success;
      setModeFromSession(session);
      await prepareDatabaseForSession(session);
      if (await blockedScopeReason(session))
        return BackgroundTask.BackgroundTaskResult.Success;
      await runSync(session);
      return BackgroundTask.BackgroundTaskResult.Success;
    } catch (error) {
      // Recovery needs the exclusive transition barrier, so release this
      // shared local-access lease before coordinating the one-time rotation.
      releaseLocalAccess?.();
      releaseLocalAccess = null;
      if (session && isSandboxGenerationRetired(error)) {
        // The task may finish after a foreground logout or mode switch. Never
        // rotate the stale token or overwrite the newer durable session.
        const durableSession = await readSession().catch(() => null);
        if (!sameSession(durableSession, session)) {
          return BackgroundTask.BackgroundTaskResult.Success;
        }

        const auth = useAuthStore.getState();
        if (auth.switchingMode) {
          const coordinated = activeRetiredSandboxRecovery(session);
          if (!coordinated) {
            return BackgroundTask.BackgroundTaskResult.Failed;
          }
          return coordinated.then(
            () => BackgroundTask.BackgroundTaskResult.Success,
            () => BackgroundTask.BackgroundTaskResult.Failed,
          );
        }
        if (auth.session && !sameSession(auth.session, session)) {
          return BackgroundTask.BackgroundTaskResult.Success;
        }

        // This also covers startup before AuthProvider hydration. The root
        // transition screen remains mounted while the coordinator clears and
        // replaces the retired Sandbox database.
        useAuthStore.setState({
          switchingMode: true,
          session: null,
          terminalEnrolled: false,
          notice: SANDBOX_RETIRED_MESSAGE,
          bootError: null,
        });
        resetSyncStateForSession(null);
        try {
          const recovered = await recoverRetiredSandboxGeneration(
            session,
            "sandbox",
            async (result) => {
              await hydrateSyncStateForSession(result.session, result.summary);
            },
          );
          useAuthStore.setState({
            session: recovered.session,
            terminalEnrolled: recovered.terminalEnrolled,
            notice: recovered.notice,
            bootError: null,
          });
          return BackgroundTask.BackgroundTaskResult.Success;
        } catch {
          useAuthStore.setState({
            session: null,
            terminalEnrolled: false,
            notice: SANDBOX_RECOVERY_ERROR,
            bootError: null,
          });
          return BackgroundTask.BackgroundTaskResult.Failed;
        } finally {
          useAuthStore.setState({ switchingMode: false });
        }
      }
      return BackgroundTask.BackgroundTaskResult.Failed;
    } finally {
      releaseLocalAccess?.();
    }
  });
}

let registrationInFlight: Promise<boolean> | null = null;

async function registerBackgroundSyncInternal(): Promise<boolean> {
  const status = await BackgroundTask.getStatusAsync();
  if (status !== BackgroundTask.BackgroundTaskStatus.Available) return false;

  if (await TaskManager.isTaskRegisteredAsync(BACKGROUND_SYNC_TASK)) {
    return true;
  }

  await BackgroundTask.registerTaskAsync(BACKGROUND_SYNC_TASK, {
    minimumInterval: BACKGROUND_SYNC_MINIMUM_INTERVAL_MINUTES,
  });
  return TaskManager.isTaskRegisteredAsync(BACKGROUND_SYNC_TASK);
}

export function registerBackgroundSync(): Promise<boolean> {
  registrationInFlight ??= registerBackgroundSyncInternal().finally(() => {
    registrationInFlight = null;
  });
  return registrationInFlight;
}
