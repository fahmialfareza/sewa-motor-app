import * as BackgroundTask from "expo-background-task";
import * as TaskManager from "expo-task-manager";

import { readSession } from "@/security/secure-store";

import { runSync } from "./engine";

export const BACKGROUND_SYNC_TASK = "sewa-motor-background-sync";
export const BACKGROUND_SYNC_MINIMUM_INTERVAL_MINUTES = 15;

if (!TaskManager.isTaskDefined(BACKGROUND_SYNC_TASK)) {
  TaskManager.defineTask(BACKGROUND_SYNC_TASK, async () => {
    try {
      const session = await readSession();
      if (!session) return BackgroundTask.BackgroundTaskResult.Success;
      await runSync(session);
      return BackgroundTask.BackgroundTaskResult.Success;
    } catch {
      return BackgroundTask.BackgroundTaskResult.Failed;
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
