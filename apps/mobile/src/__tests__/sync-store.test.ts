import type { Session } from "@/domain/types";
import {
  beginModeTransition,
  resetMutationBarrierForTests,
} from "@/mode/mutation-barrier";
import type { SyncSummary } from "@/sync/engine";
import { useSyncStore } from "@/sync/sync-store";
import { SERVER_UNREACHABLE_MESSAGE } from "@/utils/errors";

const mockRunSync = jest.fn<Promise<SyncSummary>, [Session]>();
const mockCountPendingOutbox = jest.fn<Promise<number>, []>();
const mockGetSyncMetadata = jest.fn();
const mockReadSession = jest.fn<Promise<Session | null>, []>();
const mockReadTerminalIdentity = jest.fn();
const mockSetAuthState = jest.fn();
const mockIsSandboxGenerationRetired = jest.fn();
const mockRecoverRetiredSandboxGeneration = jest.fn();
const mockActiveRetiredSandboxRecovery = jest.fn();

const mockSession: Session = {
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
const mockAuthState: {
  session: Session | null;
  switchingMode: boolean;
} = { session: mockSession, switchingMode: false };

jest.mock("@/auth/auth-store", () => ({
  useAuthStore: {
    getState: () => mockAuthState,
    setState: (state: unknown) => mockSetAuthState(state),
  },
}));

jest.mock("@/db/repositories", () => ({
  countPendingOutbox: () => mockCountPendingOutbox(),
  getSyncMetadata: () => mockGetSyncMetadata(),
}));

jest.mock("@/security/secure-store", () => ({
  readSession: () => mockReadSession(),
  readTerminalIdentity: () => mockReadTerminalIdentity(),
}));

jest.mock("@/mode/recovery", () => ({
  SANDBOX_RECOVERY_ERROR:
    "Mode Uji telah direset, tetapi pemulihan otomatis belum berhasil. Data Mode Uji lama sudah dibersihkan. Silakan masuk kembali untuk melanjutkan.",
  SANDBOX_RETIRED_MESSAGE:
    "Mode Uji telah direset oleh superadmin. Data Mode Uji lama sudah dibersihkan dari perangkat. Masuk kembali, lalu aktifkan Mode Uji untuk memuat generasi terbaru.",
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

const summary: SyncSummary = {
  pushed: 1,
  pulled: 2,
  conflicts: 0,
  completedAt: "2026-07-28T00:00:00.000Z",
};

describe("sync store concurrency", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    resetMutationBarrierForTests();
    mockAuthState.session = mockSession;
    mockAuthState.switchingMode = false;
    mockSetAuthState.mockImplementation((state: unknown) => {
      Object.assign(mockAuthState, state);
    });
    mockCountPendingOutbox.mockResolvedValue(0);
    mockGetSyncMetadata.mockResolvedValue({
      cursor: "CURSOR-1",
      lastSyncedAt: summary.completedAt,
      lastError: null,
    });
    mockRunSync.mockResolvedValue(summary);
    mockReadSession.mockResolvedValue(mockSession);
    mockReadTerminalIdentity.mockResolvedValue({
      enrolledAt: "2026-07-28T00:00:00.000Z",
    });
    mockIsSandboxGenerationRetired.mockReturnValue(false);
    mockRecoverRetiredSandboxGeneration.mockReset();
    mockActiveRetiredSandboxRecovery.mockReturnValue(null);
    useSyncStore.setState({
      online: true,
      syncing: false,
      pendingCount: 0,
      lastSyncedAt: null,
      lastError: null,
      lastSummary: null,
    });
  });

  afterEach(() => {
    resetMutationBarrierForTests();
  });

  it("shares one in-flight operation between automatic and manual callers", async () => {
    let resolveSync: (value: SyncSummary) => void = () => {
      throw new Error("Sync resolver was not initialized.");
    };
    mockRunSync.mockImplementation(
      () =>
        new Promise<SyncSummary>((resolve) => {
          resolveSync = resolve;
        }),
    );

    const first = useSyncStore.getState().syncNow();
    const second = useSyncStore.getState().syncNow();
    await Promise.resolve();

    expect(second).toBe(first);
    expect(useSyncStore.getState().syncing).toBe(true);
    expect(mockRunSync).toHaveBeenCalledTimes(1);

    resolveSync(summary);
    await expect(Promise.all([first, second])).resolves.toEqual([
      summary,
      summary,
    ]);

    expect(useSyncStore.getState()).toMatchObject({
      syncing: false,
      lastSummary: summary,
      pendingCount: 0,
      lastError: null,
    });
    expect(mockSetAuthState).toHaveBeenCalledTimes(1);
  });

  it("does not restore an old session when its deferred secure-store read finishes after a transition", async () => {
    let resolveTerminalRead: (value: { enrolledAt: string }) => void = () => {
      throw new Error("Terminal resolver was not initialized.");
    };
    let announceTerminalRead: () => void = () => undefined;
    const terminalReadStarted = new Promise<void>((resolve) => {
      announceTerminalRead = resolve;
    });
    mockReadTerminalIdentity.mockImplementationOnce(
      () =>
        new Promise<{ enrolledAt: string }>((resolve) => {
          resolveTerminalRead = resolve;
          announceTerminalRead();
        }),
    );

    const syncing = useSyncStore.getState().syncNow();
    await terminalReadStarted;

    let transitionAcquired = false;
    const transition = beginModeTransition().then((lease) => {
      transitionAcquired = true;
      return lease;
    });
    await Promise.resolve();
    expect(transitionAcquired).toBe(false);

    const replacementSession: Session = {
      ...mockSession,
      token: "replacement-token",
      sessionId: "REPLACEMENT-SESSION",
      dataMode: "sandbox",
      dataSpaceId: "00000000-0000-4000-8000-000000000207",
      sandboxGeneration: 7,
    };
    mockAuthState.session = replacementSession;
    resolveTerminalRead({ enrolledAt: "2026-07-28T00:00:00.000Z" });

    await expect(syncing).resolves.toEqual(summary);
    const transitionLease = await transition;
    transitionLease.release();
    expect(mockSetAuthState).not.toHaveBeenCalledWith(
      expect.objectContaining({ session: mockSession }),
    );
    expect(mockAuthState.session).toBe(replacementSession);
  });

  it("does not start while offline or logged out", async () => {
    useSyncStore.setState({ online: false });
    await expect(useSyncStore.getState().syncNow()).resolves.toBeNull();

    useSyncStore.setState({ online: true });
    mockAuthState.session = null;
    await expect(useSyncStore.getState().syncNow()).resolves.toBeNull();

    expect(mockRunSync).not.toHaveBeenCalled();
  });

  it("sanitizes legacy native errors restored from sync metadata", async () => {
    mockGetSyncMetadata.mockResolvedValue({
      cursor: "CURSOR-1",
      lastSyncedAt: null,
      lastError:
        "fetch failed: java.net.ConnectException: Failed to connect to /192.168.18.254:8080",
    });

    await useSyncStore.getState().refresh();

    expect(useSyncStore.getState().lastError).toBe(SERVER_UNREACHABLE_MESSAGE);
  });

  it("keeps a failed sync message understandable after refresh", async () => {
    const nativeError = new Error(
      "fetch failed: java.net.ConnectException: Failed to connect to /192.168.18.254:8080",
    );
    mockRunSync.mockRejectedValue(nativeError);
    mockGetSyncMetadata.mockResolvedValue({
      cursor: "CURSOR-1",
      lastSyncedAt: null,
      lastError: nativeError.message,
    });

    await expect(useSyncStore.getState().syncNow()).rejects.toBe(nativeError);

    expect(useSyncStore.getState().lastError).toBe(SERVER_UNREACHABLE_MESSAGE);
  });

  it("recovers the active Sandbox session into the latest generation", async () => {
    const sandboxSession: Session = {
      ...mockSession,
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
    mockAuthState.session = sandboxSession;
    mockRunSync.mockRejectedValue(retired);
    mockIsSandboxGenerationRetired.mockReturnValue(true);
    const recoveredSession: Session = {
      ...sandboxSession,
      token: "sandbox-token-8",
      sessionId: "SANDBOX-SESSION-8",
      dataSpaceId: "00000000-0000-4000-8000-000000000208",
      sandboxGeneration: 8,
    };
    const recoveredSummary: SyncSummary = {
      ...summary,
      pulled: 5,
    };
    mockRecoverRetiredSandboxGeneration.mockImplementationOnce(
      async (
        _session: Session,
        _targetMode: string,
        beforeExposure: (result: unknown) => Promise<void>,
      ) => {
        // This would deadlock if syncNow retained its shared local-access lease
        // while invoking retired-generation recovery.
        const transitionLease = await beginModeTransition();
        const result = {
          session: recoveredSession,
          summary: recoveredSummary,
          terminalEnrolled: true,
          notice: "Mode Uji otomatis dipulihkan.",
        };
        try {
          await beforeExposure(result);
          return result;
        } finally {
          transitionLease.release();
        }
      },
    );

    await expect(useSyncStore.getState().syncNow()).resolves.toEqual(
      recoveredSummary,
    );

    expect(mockRecoverRetiredSandboxGeneration).toHaveBeenCalledWith(
      sandboxSession,
      "sandbox",
      expect.any(Function),
    );
    expect(mockSetAuthState).toHaveBeenCalledWith({
      session: null,
      terminalEnrolled: false,
      notice:
        "Mode Uji telah direset oleh superadmin. Data Mode Uji lama sudah dibersihkan dari perangkat. Masuk kembali, lalu aktifkan Mode Uji untuk memuat generasi terbaru.",
      bootError: null,
    });
    expect(mockSetAuthState).toHaveBeenCalledWith({
      session: recoveredSession,
      terminalEnrolled: true,
      notice: "Mode Uji otomatis dipulihkan.",
      bootError: null,
    });
    expect(useSyncStore.getState()).toMatchObject({
      dataSpaceId: recoveredSession.dataSpaceId,
      lastSummary: recoveredSummary,
    });
  });

  it("returns to login when retired-generation recovery fails", async () => {
    const sandboxSession: Session = {
      ...mockSession,
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
    const recoveryError = new Error("Pemulihan otomatis belum berhasil");
    mockAuthState.session = sandboxSession;
    mockRunSync.mockRejectedValue(retired);
    mockIsSandboxGenerationRetired.mockReturnValue(true);
    mockRecoverRetiredSandboxGeneration.mockRejectedValueOnce(recoveryError);

    await expect(useSyncStore.getState().syncNow()).rejects.toBe(recoveryError);

    expect(mockSetAuthState).toHaveBeenCalledWith({
      session: null,
      terminalEnrolled: false,
      notice: expect.stringContaining("pemulihan otomatis belum berhasil"),
      bootError: null,
    });
    expect(mockAuthState).toMatchObject({
      session: null,
      switchingMode: false,
    });
  });

  it("joins a recovery that takes ownership while sync is in flight", async () => {
    const sandboxSession: Session = {
      ...mockSession,
      token: "sandbox-token",
      sessionId: "SANDBOX-SESSION-7",
      dataMode: "sandbox",
      dataSpaceId: "00000000-0000-4000-8000-000000000207",
      sandboxGeneration: 7,
    };
    const recoveredSummary: SyncSummary = { ...summary, pulled: 8 };
    let rejectSync: (error: unknown) => void = () => undefined;
    let announceSync: () => void = () => undefined;
    const syncStarted = new Promise<void>((resolve) => {
      announceSync = resolve;
    });
    mockAuthState.session = sandboxSession;
    mockRunSync.mockImplementationOnce(
      () =>
        new Promise<SyncSummary>((_resolve, reject) => {
          rejectSync = reject;
          announceSync();
        }),
    );
    mockIsSandboxGenerationRetired.mockReturnValue(true);

    const syncing = useSyncStore.getState().syncNow();
    await syncStarted;
    mockAuthState.switchingMode = true;
    mockActiveRetiredSandboxRecovery.mockReturnValueOnce(
      Promise.resolve({ summary: recoveredSummary }),
    );
    rejectSync({ code: "SANDBOX_GENERATION_RETIRED" });

    await expect(syncing).resolves.toEqual(recoveredSummary);
    expect(mockActiveRetiredSandboxRecovery).toHaveBeenCalledWith(
      sandboxSession,
    );
    expect(mockRecoverRetiredSandboxGeneration).not.toHaveBeenCalled();
  });
});
