import * as BackgroundTask from "expo-background-task";
import type { TaskManagerTaskExecutor } from "expo-task-manager";

import type { Session } from "@/domain/types";

const mockGetStatus = jest.fn();
const mockRegisterTask = jest.fn<
  Promise<void>,
  [string, { minimumInterval: number }]
>();
const mockIsTaskDefined = jest.fn<boolean, [string]>(() => false);
const mockIsTaskRegistered = jest.fn<Promise<boolean>, [string]>();
const mockDefineTask = jest.fn();
const mockReadSession = jest.fn<Promise<Session | null>, []>();
const mockRunSync = jest.fn();
const mockPrepareDatabaseForSession = jest.fn();
const mockSetModeFromSession = jest.fn();
const mockBeginModeSafeLocalAccess = jest.fn();
const mockReleaseLocalAccess = jest.fn();
const mockIsSandboxGenerationRetired = jest.fn();
const mockRecoverRetiredSandboxGeneration = jest.fn();
const mockActiveRetiredSandboxRecovery = jest.fn();
const mockSetAuthState = jest.fn();
const mockResetSyncStateForSession = jest.fn();
const mockHydrateSyncStateForSession = jest.fn();
const mockAuthState: { session: Session | null; switchingMode: boolean } = {
  session: null,
  switchingMode: false,
};

let mockTaskExecutor: TaskManagerTaskExecutor | null = null;

jest.mock("expo-background-task", () => ({
  BackgroundTaskResult: { Success: 1, Failed: 2 },
  BackgroundTaskStatus: { Restricted: 1, Available: 2 },
  getStatusAsync: () => mockGetStatus(),
  registerTaskAsync: (taskName: string, options: { minimumInterval: number }) =>
    mockRegisterTask(taskName, options),
}));

jest.mock("expo-task-manager", () => ({
  isTaskDefined: (taskName: string) => mockIsTaskDefined(taskName),
  defineTask: (taskName: string, executor: TaskManagerTaskExecutor) => {
    mockTaskExecutor = executor;
    mockDefineTask(taskName, executor);
  },
  isTaskRegisteredAsync: (taskName: string) => mockIsTaskRegistered(taskName),
}));

jest.mock("@/security/secure-store", () => ({
  readSession: () => mockReadSession(),
}));

jest.mock("@/auth/auth-store", () => ({
  useAuthStore: {
    getState: () => mockAuthState,
    setState: (state: unknown) => mockSetAuthState(state),
  },
}));

jest.mock("@/db/client", () => ({
  prepareDatabaseForSession: (session: Session) =>
    mockPrepareDatabaseForSession(session),
}));

jest.mock("@/mode/mode-store", () => ({
  setModeFromSession: (session: Session | null) =>
    mockSetModeFromSession(session),
}));

jest.mock("@/mode/mutation-barrier", () => ({
  beginModeSafeLocalAccess: (...args: unknown[]) =>
    mockBeginModeSafeLocalAccess(...args),
}));

jest.mock("@/mode/recovery", () => ({
  activeRetiredSandboxRecovery: (...args: unknown[]) =>
    mockActiveRetiredSandboxRecovery(...args),
  isSandboxGenerationRetired: (...args: unknown[]) =>
    mockIsSandboxGenerationRetired(...args),
  recoverRetiredSandboxGeneration: (...args: unknown[]) =>
    mockRecoverRetiredSandboxGeneration(...args),
}));

jest.mock("@/sync/engine", () => ({
  runSync: (session: Session) => mockRunSync(session),
}));

jest.mock("@/sync/state-handoff", () => ({
  resetSyncStateForSession: (...args: unknown[]) =>
    mockResetSyncStateForSession(...args),
  hydrateSyncStateForSession: (...args: unknown[]) =>
    mockHydrateSyncStateForSession(...args),
}));

// Load after mock state is initialized because the real module defines its task at import time.
const {
  BACKGROUND_SYNC_MINIMUM_INTERVAL_MINUTES,
  BACKGROUND_SYNC_TASK,
  registerBackgroundSync,
} = jest.requireActual<typeof import("@/sync/background")>("@/sync/background");

const session: Session = {
  token: "session-token",
  sessionId: "SESSION-1",
  establishedAt: "2026-07-28T00:00:00.000Z",
  dataMode: "production",
  dataSpaceId: "00000000-0000-4000-8000-000000000100",
  sandboxGeneration: null,
  user: {
    id: "USER-1",
    fullName: "Kasir",
    username: "kasir",
    role: "admin",
    active: true,
    mustChangePassword: false,
  },
};

describe("background sync", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockGetStatus.mockResolvedValue(
      BackgroundTask.BackgroundTaskStatus.Available,
    );
    mockRegisterTask.mockResolvedValue();
    mockIsTaskRegistered.mockResolvedValue(true);
    mockReadSession.mockResolvedValue(session);
    mockRunSync.mockResolvedValue({
      pushed: 0,
      pulled: 0,
      conflicts: 0,
      completedAt: "2026-07-28T00:00:00.000Z",
    });
    mockPrepareDatabaseForSession.mockResolvedValue(undefined);
    mockBeginModeSafeLocalAccess.mockResolvedValue(mockReleaseLocalAccess);
    mockIsSandboxGenerationRetired.mockReturnValue(false);
    mockRecoverRetiredSandboxGeneration.mockResolvedValue(undefined);
    mockActiveRetiredSandboxRecovery.mockReturnValue(null);
    mockAuthState.session = null;
    mockAuthState.switchingMode = false;
    mockSetAuthState.mockImplementation((state: unknown) => {
      Object.assign(mockAuthState, state);
    });
    mockResetSyncStateForSession.mockReturnValue(undefined);
    mockHydrateSyncStateForSession.mockResolvedValue(undefined);
  });

  it("defines the native task in module scope", () => {
    expect(mockTaskExecutor).toEqual(expect.any(Function));
  });

  it("registers the task at the platform minimum and verifies persistence", async () => {
    mockIsTaskRegistered
      .mockResolvedValueOnce(false)
      .mockResolvedValueOnce(true);

    await expect(registerBackgroundSync()).resolves.toBe(true);

    expect(mockRegisterTask).toHaveBeenCalledWith(BACKGROUND_SYNC_TASK, {
      minimumInterval: BACKGROUND_SYNC_MINIMUM_INTERVAL_MINUTES,
    });
    expect(mockIsTaskRegistered).toHaveBeenCalledTimes(2);
  });

  it("does not duplicate an existing native registration", async () => {
    await expect(registerBackgroundSync()).resolves.toBe(true);
    expect(mockRegisterTask).not.toHaveBeenCalled();
  });

  it("skips registration when background execution is restricted", async () => {
    mockGetStatus.mockResolvedValue(
      BackgroundTask.BackgroundTaskStatus.Restricted,
    );

    await expect(registerBackgroundSync()).resolves.toBe(false);
    expect(mockIsTaskRegistered).not.toHaveBeenCalled();
    expect(mockRegisterTask).not.toHaveBeenCalled();
  });

  it("coalesces concurrent registration attempts", async () => {
    let resolveStatus: (value: number) => void = () => {
      throw new Error("Status resolver was not initialized.");
    };
    mockGetStatus.mockImplementation(
      () =>
        new Promise<number>((resolve) => {
          resolveStatus = resolve;
        }),
    );

    const first = registerBackgroundSync();
    const second = registerBackgroundSync();
    expect(second).toBe(first);

    resolveStatus(BackgroundTask.BackgroundTaskStatus.Available);
    await Promise.all([first, second]);
    expect(mockGetStatus).toHaveBeenCalledTimes(1);
  });

  it("returns success without a session and runs sync with a session", async () => {
    if (!mockTaskExecutor) throw new Error("Background task was not defined.");
    mockReadSession.mockResolvedValueOnce(null).mockResolvedValueOnce(session);

    await expect(
      mockTaskExecutor({
        data: undefined,
        error: null,
        executionInfo: {
          eventId: "EVENT-1",
          taskName: BACKGROUND_SYNC_TASK,
        },
      }),
    ).resolves.toBe(BackgroundTask.BackgroundTaskResult.Success);
    expect(mockRunSync).not.toHaveBeenCalled();

    await expect(
      mockTaskExecutor({
        data: undefined,
        error: null,
        executionInfo: {
          eventId: "EVENT-2",
          taskName: BACKGROUND_SYNC_TASK,
        },
      }),
    ).resolves.toBe(BackgroundTask.BackgroundTaskResult.Success);
    expect(mockRunSync).toHaveBeenCalledWith(session);
    expect(mockSetModeFromSession).toHaveBeenCalledWith(session);
    expect(mockPrepareDatabaseForSession).toHaveBeenCalledWith(session);
    expect(mockReleaseLocalAccess).toHaveBeenCalledTimes(2);
  });

  it("waits for mode-safe access before reading or syncing a headless session", async () => {
    if (!mockTaskExecutor) throw new Error("Background task was not defined.");
    let grantAccess: (release: () => void) => void = () => undefined;
    mockBeginModeSafeLocalAccess.mockImplementationOnce(
      () =>
        new Promise<() => void>((resolve) => {
          grantAccess = resolve;
        }),
    );

    const executing = mockTaskExecutor({
      data: undefined,
      error: null,
      executionInfo: {
        eventId: "EVENT-WAIT-FOR-MODE",
        taskName: BACKGROUND_SYNC_TASK,
      },
    });
    await Promise.resolve();

    expect(mockReadSession).not.toHaveBeenCalled();
    expect(mockPrepareDatabaseForSession).not.toHaveBeenCalled();
    expect(mockRunSync).not.toHaveBeenCalled();

    const replacementSession: Session = {
      ...session,
      token: "replacement-token",
      sessionId: "REPLACEMENT-SESSION",
    };
    mockReadSession.mockResolvedValue(replacementSession);
    grantAccess(mockReleaseLocalAccess);
    await expect(executing).resolves.toBe(
      BackgroundTask.BackgroundTaskResult.Success,
    );
    expect(mockReadSession).toHaveBeenCalledTimes(1);
    expect(mockRunSync).toHaveBeenCalledWith(replacementSession);
    expect(mockRunSync).not.toHaveBeenCalledWith(session);
    expect(mockReleaseLocalAccess).toHaveBeenCalledTimes(1);
  });

  it("reports native task failure when session read or sync fails", async () => {
    if (!mockTaskExecutor) throw new Error("Background task was not defined.");
    mockReadSession.mockRejectedValueOnce(
      new Error("secure store unavailable"),
    );

    await expect(
      mockTaskExecutor({
        data: undefined,
        error: null,
        executionInfo: {
          eventId: "EVENT-3",
          taskName: BACKGROUND_SYNC_TASK,
        },
      }),
    ).resolves.toBe(BackgroundTask.BackgroundTaskResult.Failed);

    mockReadSession.mockResolvedValueOnce(session);
    mockRunSync.mockRejectedValueOnce(new Error("network failed"));
    await expect(
      mockTaskExecutor({
        data: undefined,
        error: null,
        executionInfo: {
          eventId: "EVENT-4",
          taskName: BACKGROUND_SYNC_TASK,
        },
      }),
    ).resolves.toBe(BackgroundTask.BackgroundTaskResult.Failed);
  });

  it("recovers a stale Sandbox session after background sync detects a reset", async () => {
    if (!mockTaskExecutor) throw new Error("Background task was not defined.");
    const sandboxSession: Session = {
      ...session,
      token: "sandbox-token",
      sessionId: "SANDBOX-SESSION-7",
      dataMode: "sandbox",
      dataSpaceId: "00000000-0000-4000-8000-000000000207",
      sandboxGeneration: 7,
    };
    const retired = {
      code: "SANDBOX_GENERATION_RETIRED",
      message: "Generasi Mode Uji telah direset.",
    };
    mockReadSession
      .mockResolvedValueOnce(sandboxSession)
      .mockResolvedValueOnce(sandboxSession);
    mockRunSync.mockRejectedValueOnce(retired);
    mockIsSandboxGenerationRetired.mockReturnValueOnce(true);
    const recoveredSession: Session = {
      ...sandboxSession,
      token: "sandbox-token-8",
      sessionId: "SANDBOX-SESSION-8",
      dataSpaceId: "00000000-0000-4000-8000-000000000208",
      sandboxGeneration: 8,
    };
    const recoveredSummary = {
      pushed: 0,
      pulled: 3,
      conflicts: 0,
      completedAt: "2026-07-28T01:00:00.000Z",
    };
    mockRecoverRetiredSandboxGeneration.mockImplementationOnce(
      async (
        _session: Session,
        _targetMode: string,
        beforeExposure: (result: unknown) => Promise<void>,
      ) => {
        const result = {
          session: recoveredSession,
          summary: recoveredSummary,
          terminalEnrolled: true,
          notice: "Mode Uji otomatis dipulihkan.",
        };
        await beforeExposure(result);
        return result;
      },
    );

    await expect(
      mockTaskExecutor({
        data: undefined,
        error: null,
        executionInfo: {
          eventId: "EVENT-SANDBOX-RETIRED",
          taskName: BACKGROUND_SYNC_TASK,
        },
      }),
    ).resolves.toBe(BackgroundTask.BackgroundTaskResult.Success);

    expect(mockRecoverRetiredSandboxGeneration).toHaveBeenCalledWith(
      sandboxSession,
      "sandbox",
      expect.any(Function),
    );
    expect(mockHydrateSyncStateForSession).toHaveBeenCalledWith(
      recoveredSession,
      recoveredSummary,
    );
    expect(mockAuthState).toMatchObject({
      session: recoveredSession,
      switchingMode: false,
    });
  });

  it("reports failure when background retired-generation recovery cannot finish", async () => {
    if (!mockTaskExecutor) throw new Error("Background task was not defined.");
    const sandboxSession: Session = {
      ...session,
      token: "sandbox-token",
      sessionId: "SANDBOX-SESSION-7",
      dataMode: "sandbox",
      dataSpaceId: "00000000-0000-4000-8000-000000000207",
      sandboxGeneration: 7,
    };
    mockReadSession
      .mockResolvedValueOnce(sandboxSession)
      .mockResolvedValueOnce(sandboxSession);
    mockRunSync.mockRejectedValueOnce({
      code: "SANDBOX_GENERATION_RETIRED",
    });
    mockIsSandboxGenerationRetired.mockReturnValueOnce(true);
    mockRecoverRetiredSandboxGeneration.mockRejectedValueOnce(
      new Error("recovery failed"),
    );

    await expect(
      mockTaskExecutor({
        data: undefined,
        error: null,
        executionInfo: {
          eventId: "EVENT-SANDBOX-RECOVERY-FAILED",
          taskName: BACKGROUND_SYNC_TASK,
        },
      }),
    ).resolves.toBe(BackgroundTask.BackgroundTaskResult.Failed);
  });

  it("does not overwrite a newer durable session after a stale task finishes", async () => {
    if (!mockTaskExecutor) throw new Error("Background task was not defined.");
    const sandboxSession: Session = {
      ...session,
      token: "sandbox-token",
      sessionId: "SANDBOX-SESSION-7",
      dataMode: "sandbox",
      dataSpaceId: "00000000-0000-4000-8000-000000000207",
      sandboxGeneration: 7,
    };
    mockReadSession
      .mockResolvedValueOnce(sandboxSession)
      .mockResolvedValueOnce(session);
    mockRunSync.mockRejectedValueOnce({
      code: "SANDBOX_GENERATION_RETIRED",
    });
    mockIsSandboxGenerationRetired.mockReturnValueOnce(true);

    await expect(
      mockTaskExecutor({
        data: undefined,
        error: null,
        executionInfo: {
          eventId: "EVENT-STALE-SANDBOX-TASK",
          taskName: BACKGROUND_SYNC_TASK,
        },
      }),
    ).resolves.toBe(BackgroundTask.BackgroundTaskResult.Success);

    expect(mockRecoverRetiredSandboxGeneration).not.toHaveBeenCalled();
    expect(mockSetAuthState).not.toHaveBeenCalled();
  });

  it("joins an in-flight foreground recovery instead of rotating again", async () => {
    if (!mockTaskExecutor) throw new Error("Background task was not defined.");
    const sandboxSession: Session = {
      ...session,
      token: "sandbox-token",
      sessionId: "SANDBOX-SESSION-7",
      dataMode: "sandbox",
      dataSpaceId: "00000000-0000-4000-8000-000000000207",
      sandboxGeneration: 7,
    };
    mockReadSession
      .mockResolvedValueOnce(sandboxSession)
      .mockResolvedValueOnce(sandboxSession);
    mockRunSync.mockRejectedValueOnce({
      code: "SANDBOX_GENERATION_RETIRED",
    });
    mockIsSandboxGenerationRetired.mockReturnValueOnce(true);
    mockAuthState.session = sandboxSession;
    mockAuthState.switchingMode = true;
    mockActiveRetiredSandboxRecovery.mockReturnValueOnce(
      Promise.resolve({ summary: { pulled: 2 } }),
    );

    await expect(
      mockTaskExecutor({
        data: undefined,
        error: null,
        executionInfo: {
          eventId: "EVENT-JOIN-FOREGROUND-RECOVERY",
          taskName: BACKGROUND_SYNC_TASK,
        },
      }),
    ).resolves.toBe(BackgroundTask.BackgroundTaskResult.Success);

    expect(mockActiveRetiredSandboxRecovery).toHaveBeenCalledWith(
      sandboxSession,
    );
    expect(mockRecoverRetiredSandboxGeneration).not.toHaveBeenCalled();
  });
});
