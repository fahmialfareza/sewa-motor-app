import { act, render } from "@testing-library/react-native";
import type { ReactNode } from "react";
import { AppState, Text, type AppStateStatus } from "react-native";

import { FOREGROUND_SYNC_INTERVAL_MS, SyncProvider } from "@/sync/SyncProvider";

const mockRefresh = jest.fn<Promise<void>, []>();
const mockSyncNow = jest.fn<Promise<null>, []>();
const mockSetOnline = jest.fn<void, [boolean]>();
const mockRegisterBackgroundSync = jest.fn<Promise<boolean>, []>();
const mockUnsubscribeNetwork = jest.fn();
const mockRemoveAppStateListener = jest.fn();

const mockAuthState: { session: { sessionId: string } | null } = {
  session: { sessionId: "SESSION-1" },
};
const mockSyncState = {
  online: true,
  syncing: false,
  pendingCount: 0,
  lastSyncedAt: null,
  lastError: null,
  lastSummary: null,
  refresh: mockRefresh,
  syncNow: mockSyncNow,
  setOnline: mockSetOnline,
};

let mockNetworkListener:
  | ((state: {
      isConnected: boolean | null;
      isInternetReachable: boolean | null;
    }) => void)
  | null = null;
let mockAppStateListener: ((state: AppStateStatus) => void) | null = null;
const originalAppState = AppState.currentState;

jest.mock("@react-native-community/netinfo", () => ({
  __esModule: true,
  default: {
    addEventListener: jest.fn(
      (
        listener: (state: {
          isConnected: boolean | null;
          isInternetReachable: boolean | null;
        }) => void,
      ) => {
        mockNetworkListener = listener;
        return mockUnsubscribeNetwork;
      },
    ),
  },
}));

jest.mock("@/auth/auth-store", () => {
  const useAuthStore = (selector: (state: typeof mockAuthState) => unknown) =>
    selector(mockAuthState);
  useAuthStore.getState = () => mockAuthState;
  return { useAuthStore };
});

jest.mock("@/sync/sync-store", () => {
  const useSyncStore = (selector: (state: typeof mockSyncState) => unknown) =>
    selector(mockSyncState);
  useSyncStore.getState = () => mockSyncState;
  return { useSyncStore };
});

jest.mock("@/sync/background", () => ({
  registerBackgroundSync: () => mockRegisterBackgroundSync(),
}));

function renderProvider(children: ReactNode = <Text>Content</Text>) {
  return render(<SyncProvider>{children}</SyncProvider>);
}

describe("SyncProvider auto-sync lifecycle", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    jest.clearAllMocks();
    mockAuthState.session = { sessionId: "SESSION-1" };
    mockSyncState.online = true;
    mockNetworkListener = null;
    mockAppStateListener = null;
    mockRefresh.mockResolvedValue();
    mockSyncNow.mockResolvedValue(null);
    mockRegisterBackgroundSync.mockResolvedValue(true);
    mockSetOnline.mockImplementation((online) => {
      mockSyncState.online = online;
    });
    jest
      .spyOn(AppState, "addEventListener")
      .mockImplementation((_type, listener) => {
        mockAppStateListener = listener;
        return { remove: mockRemoveAppStateListener };
      });
    Object.defineProperty(AppState, "currentState", {
      configurable: true,
      value: "active",
    });
  });

  afterEach(() => {
    jest.restoreAllMocks();
    Object.defineProperty(AppState, "currentState", {
      configurable: true,
      value: originalAppState,
    });
    jest.useRealTimers();
  });

  it("syncs after login and periodically while the app remains active", async () => {
    const screen = renderProvider();
    await act(async () => undefined);

    expect(mockRefresh).toHaveBeenCalledTimes(1);
    expect(mockRegisterBackgroundSync).toHaveBeenCalledTimes(1);
    expect(mockSyncNow).toHaveBeenCalledTimes(1);

    await act(async () => {
      jest.advanceTimersByTime(FOREGROUND_SYNC_INTERVAL_MS * 2);
    });

    expect(mockSyncNow).toHaveBeenCalledTimes(3);
    screen.unmount();
    expect(mockUnsubscribeNetwork).toHaveBeenCalledTimes(1);
    expect(mockRemoveAppStateListener).toHaveBeenCalledTimes(1);
  });

  it("pauses the foreground interval outside active state", async () => {
    renderProvider();
    await act(async () => undefined);
    mockSyncNow.mockClear();

    await act(async () => {
      mockAppStateListener?.("background");
      jest.advanceTimersByTime(FOREGROUND_SYNC_INTERVAL_MS * 2);
    });

    expect(mockSyncNow).not.toHaveBeenCalled();
  });

  it("syncs when connectivity recovers and when the app resumes", async () => {
    renderProvider();
    await act(async () => undefined);
    mockSyncNow.mockClear();

    act(() => {
      mockNetworkListener?.({
        isConnected: false,
        isInternetReachable: false,
      });
    });
    expect(mockSyncState.online).toBe(false);

    act(() => {
      mockNetworkListener?.({
        isConnected: true,
        isInternetReachable: true,
      });
    });
    expect(mockSyncNow).toHaveBeenCalledTimes(1);

    await act(async () => {
      mockAppStateListener?.("background");
      mockAppStateListener?.("active");
    });

    expect(mockSyncNow).toHaveBeenCalledTimes(2);
    expect(mockRegisterBackgroundSync).toHaveBeenCalledTimes(2);
  });

  it("does not auto-sync without a session or while offline", async () => {
    mockAuthState.session = null;
    mockSyncState.online = false;
    renderProvider();
    await act(async () => undefined);

    await act(async () => {
      jest.advanceTimersByTime(FOREGROUND_SYNC_INTERVAL_MS);
    });

    expect(mockSyncNow).not.toHaveBeenCalled();
  });
});
