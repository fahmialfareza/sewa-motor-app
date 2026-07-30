import type { Session } from "@/domain/types";
import type { SyncSummary } from "@/sync/engine";
import { useSyncStore } from "@/sync/sync-store";
import { SERVER_UNREACHABLE_MESSAGE } from "@/utils/errors";

const mockRunSync = jest.fn<Promise<SyncSummary>, [Session]>();
const mockCountPendingOutbox = jest.fn<Promise<number>, []>();
const mockGetSyncMetadata = jest.fn();
const mockReadSession = jest.fn<Promise<Session | null>, []>();
const mockReadTerminalIdentity = jest.fn();
const mockSetAuthState = jest.fn();

const mockSession: Session = {
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
const mockAuthState: { session: Session | null } = { session: mockSession };

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
    mockAuthState.session = mockSession;
    mockCountPendingOutbox.mockResolvedValue(0);
    mockGetSyncMetadata.mockResolvedValue({
      cursor: "CURSOR-1",
      lastSyncedAt: summary.completedAt,
      lastError: null,
    });
    mockReadSession.mockResolvedValue(mockSession);
    mockReadTerminalIdentity.mockResolvedValue({
      enrolledAt: "2026-07-28T00:00:00.000Z",
    });
    useSyncStore.setState({
      online: true,
      syncing: false,
      pendingCount: 0,
      lastSyncedAt: null,
      lastError: null,
      lastSummary: null,
    });
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
});
