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

jest.mock("@/sync/engine", () => ({
  runSync: (session: Session) => mockRunSync(session),
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
});
