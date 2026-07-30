import { act, fireEvent, render, waitFor } from "@testing-library/react-native";
import type { ReactNode } from "react";

import HomeScreen from "@/app/(app)/(tabs)/home";
import type { DashboardStats } from "@/domain/types";

const mockGetDashboardStats = jest.fn<Promise<DashboardStats>, [string]>();
const mockListTransactions = jest.fn();
const mockRouterPush = jest.fn();
const mockSyncRuntime = {
  lastSyncedAt: null as string | null,
  pendingCount: 0,
};

jest.mock("expo-router", () => {
  const React = jest.requireActual<typeof import("react")>("react");
  return {
    useRouter: () => ({ push: mockRouterPush }),
    useFocusEffect: (effect: () => void | (() => void)) =>
      React.useEffect(effect, [effect]),
  };
});

jest.mock("@/auth/AuthProvider", () => ({
  useAuth: () => ({
    session: {
      user: { fullName: "Andi" },
    },
  }),
}));

jest.mock("@/sync/SyncProvider", () => ({
  useSyncRuntime: () => mockSyncRuntime,
}));

jest.mock("@/db/repositories", () => ({
  getDashboardStats: (period: string) => mockGetDashboardStats(period),
  listTransactions: (...args: unknown[]) => mockListTransactions(...args),
}));

jest.mock("@/components/layout/AppScreen", () => {
  const { View } =
    jest.requireActual<typeof import("react-native")>("react-native");
  return {
    AppScreen: ({ children }: { children: ReactNode }) => (
      <View>{children}</View>
    ),
  };
});

jest.mock("@/components/layout/PageHeader", () => {
  const { Text } =
    jest.requireActual<typeof import("react-native")>("react-native");
  return {
    PageHeader: ({ title }: { title: string }) => <Text>{title}</Text>,
  };
});

jest.mock("@/components/transactions/TransactionRow", () => ({
  TransactionRow: () => null,
}));

jest.mock("@/components/ui/Card", () => {
  const { View } =
    jest.requireActual<typeof import("react-native")>("react-native");
  return {
    Card: ({ children }: { children: ReactNode }) => <View>{children}</View>,
  };
});

jest.mock("@/components/ui/Button", () => {
  const { Pressable, Text } =
    jest.requireActual<typeof import("react-native")>("react-native");
  return {
    Button: ({
      children,
      onPress,
    }: {
      children: ReactNode;
      onPress: () => void;
    }) => (
      <Pressable accessibilityRole="button" onPress={onPress}>
        <Text>{children}</Text>
      </Pressable>
    ),
  };
});

const emptyStats: DashboardStats = {
  gross: 0,
  transactionCount: 0,
  quantities: [],
  buckets: Array(24).fill(0),
};

const populatedStats: DashboardStats = {
  gross: 170_000,
  transactionCount: 2,
  quantities: [{ name: "Paket Standar", quantity: 3, accent: "standard" }],
  buckets: [...Array(23).fill(0), 170_000],
};

describe("Home dashboard refresh", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockSyncRuntime.lastSyncedAt = null;
    mockSyncRuntime.pendingCount = 0;
    mockListTransactions.mockResolvedValue([]);
  });

  it("reloads while focused after sync and uses the live outbox count", async () => {
    mockGetDashboardStats
      .mockResolvedValueOnce(emptyStats)
      .mockResolvedValueOnce(populatedStats);
    const screen = render(<HomeScreen />);

    await waitFor(() => {
      expect(screen.getByText("0 transaksi lunas")).toBeTruthy();
    });

    mockSyncRuntime.lastSyncedAt = "2026-07-29T11:00:00.000Z";
    mockSyncRuntime.pendingCount = 2;
    screen.rerender(<HomeScreen />);

    await waitFor(() => {
      expect(screen.getByText(/170\.000/)).toBeTruthy();
      expect(screen.getByText("2 transaksi lunas")).toBeTruthy();
      expect(screen.getByText("3 unit")).toBeTruthy();
      expect(screen.getByText("2 perubahan")).toBeTruthy();
    });
    expect(mockGetDashboardStats).toHaveBeenCalledTimes(2);
  });

  it("ignores an older zero response that finishes after a synced response", async () => {
    let resolveInitial: (value: DashboardStats) => void = () => {
      throw new Error("Initial dashboard resolver was not initialized.");
    };
    mockGetDashboardStats
      .mockImplementationOnce(
        () =>
          new Promise<DashboardStats>((resolve) => {
            resolveInitial = resolve;
          }),
      )
      .mockResolvedValueOnce(populatedStats);
    const screen = render(<HomeScreen />);

    await waitFor(() => {
      expect(mockGetDashboardStats).toHaveBeenCalledTimes(1);
    });

    mockSyncRuntime.lastSyncedAt = "2026-07-29T11:00:00.000Z";
    screen.rerender(<HomeScreen />);

    await waitFor(() => {
      expect(screen.getByText("2 transaksi lunas")).toBeTruthy();
    });

    await act(async () => {
      resolveInitial(emptyStats);
    });

    expect(screen.getByText("2 transaksi lunas")).toBeTruthy();
    expect(screen.queryByText("0 transaksi lunas")).toBeNull();
  });

  it("shows a retry instead of silently keeping the initial zero state", async () => {
    mockGetDashboardStats
      .mockRejectedValueOnce(new Error("Database sedang sibuk."))
      .mockResolvedValueOnce(populatedStats);
    const screen = render(<HomeScreen />);

    await waitFor(() => {
      expect(screen.getByText("Ringkasan belum diperbarui")).toBeTruthy();
      expect(screen.getByText("Database sedang sibuk.")).toBeTruthy();
    });

    fireEvent.press(screen.getByRole("button", { name: "Coba lagi" }));

    await waitFor(() => {
      expect(screen.getByText("2 transaksi lunas")).toBeTruthy();
      expect(screen.queryByText("Ringkasan belum diperbarui")).toBeNull();
    });
  });
});
