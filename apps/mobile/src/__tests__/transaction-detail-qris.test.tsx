import { act, fireEvent, render, waitFor } from "@testing-library/react-native";
import type { ReactNode } from "react";

import TransactionDetailScreen from "@/app/(app)/transactions/[id]";
import type { Session, Transaction } from "@/domain/types";
import type { QrisConfig } from "@/security/secure-store";

const STATIC_QRIS =
  "00020101021126320014ID.CO.TEST.WWW011012345678905204729953033605802ID5910SEWA MOTOR6008DENPASAR6304AA64";
const DYNAMIC_QRIS_70K =
  "00020101021226320014ID.CO.TEST.WWW011012345678905204729953033605405700005802ID5910SEWA MOTOR6008DENPASAR63048886";
const QRIS_PAYLOAD_HASH =
  "9185bbfe94bb008d611da515fc94c2f3ad5f0c3fbfe278d8bdb463f9ce1cf500";

interface DynamicQrisCardProps {
  amount: number;
  error: string | null;
  merchantCity: string | null;
  merchantName: string | null;
  onConfigure?: () => void;
  payload: string | null;
}

const mockRouterPush = jest.fn();
const mockGetTransaction = jest.fn<Promise<Transaction | null>, [string]>();
const mockHasTerminalTransactionBlock = jest.fn<Promise<boolean>, [string]>();
const mockSetPaymentStatus = jest.fn();
const mockReadQrisConfig = jest.fn<Promise<QrisConfig | null>, []>();
const mockDynamicQrisCard = jest.fn<void, [DynamicQrisCardProps]>();
const mockSyncRefresh = jest.fn();
const mockSyncNow = jest.fn();
let mockSetSyncLastSummary:
  ((summary: { completedAt: string } | null) => void) | null = null;
const mockSyncRuntime: {
  lastSummary: { completedAt: string } | null;
  refresh: typeof mockSyncRefresh;
  syncNow: typeof mockSyncNow;
} = {
  lastSummary: null,
  refresh: mockSyncRefresh,
  syncNow: mockSyncNow,
};

const mockAuthState: { session: Session | null } = {
  session: null,
};

jest.mock("expo-router", () => {
  const React = jest.requireActual<typeof import("react")>("react");
  return {
    useFocusEffect: (effect: () => void | (() => void)) =>
      React.useEffect(effect, [effect]),
    useLocalSearchParams: () => ({ id: "TX-QRIS-1" }),
    useRouter: () => ({ push: mockRouterPush }),
  };
});

jest.mock("@/auth/AuthProvider", () => ({
  useAuth: () => mockAuthState,
}));

jest.mock("@/sync/SyncProvider", () => ({
  useSyncRuntime: () => {
    const React = jest.requireActual<typeof import("react")>("react");
    const [lastSummary, setLastSummary] = React.useState(
      mockSyncRuntime.lastSummary,
    );
    mockSetSyncLastSummary = setLastSummary;
    return { ...mockSyncRuntime, lastSummary };
  },
}));

jest.mock("expo-crypto", () => ({
  CryptoDigestAlgorithm: { SHA256: "SHA-256" },
  CryptoEncoding: { HEX: "hex" },
  digestStringAsync: jest.fn(async () =>
    Promise.resolve(
      "9185bbfe94bb008d611da515fc94c2f3ad5f0c3fbfe278d8bdb463f9ce1cf500",
    ),
  ),
}));

jest.mock("@/db/repositories", () => ({
  getTransaction: (id: string) => mockGetTransaction(id),
  hasTerminalTransactionBlock: (id: string) =>
    mockHasTerminalTransactionBlock(id),
  setPaymentStatus: (...args: unknown[]) => mockSetPaymentStatus(...args),
}));

jest.mock("@/security/secure-store", () => ({
  readQrisConfig: () => mockReadQrisConfig(),
}));

jest.mock("@/components/layout/AppScreen", () => {
  const { View } =
    jest.requireActual<typeof import("react-native")>("react-native");
  return {
    AppScreen: ({ children }: { children?: ReactNode }) => (
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

jest.mock("@/components/payments/DynamicQrisCard", () => {
  const { Pressable, Text, View } =
    jest.requireActual<typeof import("react-native")>("react-native");
  return {
    DynamicQrisCard: (props: DynamicQrisCardProps) => {
      mockDynamicQrisCard(props);
      return (
        <View testID="dynamic-qris-card">
          {props.payload ? (
            <Text>QRIS nominal tersedia</Text>
          ) : (
            <Text accessibilityRole="alert">{props.error}</Text>
          )}
          {props.onConfigure ? (
            <Pressable accessibilityRole="button" onPress={props.onConfigure}>
              <Text>Atur QRIS merchant</Text>
            </Pressable>
          ) : null}
        </View>
      );
    },
  };
});

jest.mock("@/components/ui/Button", () => {
  const { Pressable, Text } =
    jest.requireActual<typeof import("react-native")>("react-native");
  return {
    Button: ({
      children,
      disabled,
      onPress,
    }: {
      children: ReactNode;
      disabled?: boolean;
      onPress?: () => void;
    }) => (
      <Pressable
        accessibilityRole="button"
        disabled={disabled}
        onPress={onPress}
      >
        <Text>{children}</Text>
      </Pressable>
    ),
  };
});

jest.mock("@/components/ui/Card", () => {
  const { View } =
    jest.requireActual<typeof import("react-native")>("react-native");
  return {
    Card: ({ children }: { children?: ReactNode }) => <View>{children}</View>,
  };
});

jest.mock("@/components/ui/PaymentBadge", () => ({
  PaymentMethodBadge: () => null,
  PaymentStatusBadge: () => null,
}));

jest.mock("@/components/ui/StateView", () => ({
  StateView: () => null,
}));

jest.mock("@/components/ui/StatusBadge", () => ({
  StatusBadge: () => null,
}));

function session(role: Session["user"]["role"]): Session {
  return {
    token: "token",
    sessionId: "SESSION-1",
    user: {
      id: "USER-1",
      fullName: role === "superadmin" ? "Super Admin" : "Admin",
      username: role,
      role,
      active: true,
      mustChangePassword: false,
    },
    establishedAt: "2026-07-30T01:00:00.000Z",
  };
}

function transaction(overrides: Partial<Transaction> = {}): Transaction {
  return {
    id: "TX-QRIS-1",
    revision: 3,
    occurredAt: "2026-07-30T01:00:00.000Z",
    subtotal: 70_000,
    total: 70_000,
    originActorId: "USER-1",
    originActorName: "Admin",
    updatedActorName: "Admin",
    terminalId: "TERMINAL-1",
    syncState: "synced",
    printState: "pending",
    paymentMethod: "qris",
    paymentStatus: "pending",
    paymentConfirmedRevision: null,
    qrisPayloadHash: QRIS_PAYLOAD_HASH,
    deletedAt: null,
    items: [
      {
        id: "ITEM-1",
        packageId: "PACKAGE-1",
        packageRevision: 2,
        name: "Paket Harian",
        description: "Paket uji",
        accent: "standard",
        unitPrice: 70_000,
        quantity: 1,
        lineTotal: 70_000,
      },
    ],
    ...overrides,
  };
}

function latestQrisProps(): DynamicQrisCardProps {
  const props = mockDynamicQrisCard.mock.calls.at(-1)?.[0];
  if (!props) throw new Error("DynamicQrisCard was not rendered.");
  return props;
}

describe("Transaction detail dynamic QRIS", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockSyncRuntime.lastSummary = null;
    mockSetSyncLastSummary = null;
    mockAuthState.session = session("admin");
    mockHasTerminalTransactionBlock.mockResolvedValue(false);
    mockReadQrisConfig.mockResolvedValue({ staticPayload: STATIC_QRIS });
  });

  it("passes the exact amount-specific payload for a pending QRIS payment", async () => {
    mockGetTransaction.mockResolvedValue(transaction());
    const screen = render(<TransactionDetailScreen />);

    await waitFor(() => {
      expect(screen.getByTestId("dynamic-qris-card")).toBeTruthy();
    });

    expect(latestQrisProps()).toMatchObject({
      amount: 70_000,
      error: null,
      merchantCity: "DENPASAR",
      merchantName: "SEWA MOTOR",
      payload: DYNAMIC_QRIS_70K,
    });
    expect(screen.queryByRole("button", { name: "Cetak struk" })).toBeNull();
  });

  it("hides QRIS and exposes printing after success for the current revision", async () => {
    mockGetTransaction.mockResolvedValue(
      transaction({
        paymentStatus: "success",
        paymentConfirmedRevision: 3,
      }),
    );
    const screen = render(<TransactionDetailScreen />);

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Cetak struk" })).toBeTruthy();
    });

    expect(screen.queryByTestId("dynamic-qris-card")).toBeNull();
    expect(mockDynamicQrisCard).not.toHaveBeenCalled();
  });

  it("offers missing QRIS configuration only to a superadmin", async () => {
    mockAuthState.session = session("superadmin");
    mockGetTransaction.mockResolvedValue(transaction());
    mockReadQrisConfig.mockResolvedValue(null);
    const superadminScreen = render(<TransactionDetailScreen />);

    await waitFor(() => {
      expect(superadminScreen.getByRole("alert").props.children).toBe(
        "QRIS merchant belum dikonfigurasi pada perangkat ini.",
      );
    });

    expect(latestQrisProps()).toMatchObject({
      amount: 70_000,
      payload: null,
      merchantName: null,
      merchantCity: null,
    });
    fireEvent.press(
      superadminScreen.getByRole("button", {
        name: "Atur QRIS merchant",
      }),
    );
    expect(mockRouterPush).toHaveBeenCalledWith("/settings/qris");

    superadminScreen.unmount();
    jest.clearAllMocks();
    mockAuthState.session = session("admin");
    mockHasTerminalTransactionBlock.mockResolvedValue(false);
    mockGetTransaction.mockResolvedValue(transaction());
    mockReadQrisConfig.mockResolvedValue(null);
    const adminScreen = render(<TransactionDetailScreen />);

    await waitFor(() => {
      expect(adminScreen.getByRole("alert").props.children).toBe(
        "QRIS merchant belum dikonfigurasi pada perangkat ini.",
      );
    });

    expect(latestQrisProps().onConfigure).toBeUndefined();
    expect(
      adminScreen.queryByRole("button", { name: "Atur QRIS merchant" }),
    ).toBeNull();
  });

  it("refuses to regenerate QRIS with a different merchant payload", async () => {
    mockGetTransaction.mockResolvedValue(
      transaction({
        qrisPayloadHash:
          "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
      }),
    );
    const screen = render(<TransactionDetailScreen />);

    await waitFor(() => {
      expect(screen.getByRole("alert").props.children).toContain(
        "berbeda dari QRIS yang terikat",
      );
    });

    expect(latestQrisProps()).toMatchObject({
      payload: null,
      merchantName: null,
      merchantCity: null,
    });
  });

  it("does not regenerate an unbound legacy QRIS transaction", async () => {
    mockGetTransaction.mockResolvedValue(
      transaction({ qrisPayloadHash: null }),
    );
    const screen = render(<TransactionDetailScreen />);

    await waitFor(() => {
      expect(screen.getByRole("alert").props.children).toContain(
        "tidak memiliki fingerprint merchant",
      );
    });

    expect(latestQrisProps()).toMatchObject({ payload: null });
  });

  it("reloads and removes QRIS after a sync completion archives the create", async () => {
    mockGetTransaction.mockResolvedValue(transaction());
    const screen = render(<TransactionDetailScreen />);
    await waitFor(() => {
      expect(screen.getByTestId("dynamic-qris-card")).toBeTruthy();
    });
    const loadCountBeforeSync = mockGetTransaction.mock.calls.length;

    mockGetTransaction.mockResolvedValue(
      transaction({ deletedAt: "2026-07-30T02:00:00.000Z" }),
    );
    mockHasTerminalTransactionBlock.mockResolvedValue(true);
    await act(async () => {
      mockSetSyncLastSummary?.({
        completedAt: "2026-07-30T02:00:01.000Z",
      });
    });

    await waitFor(() => {
      expect(mockGetTransaction.mock.calls.length).toBeGreaterThan(
        loadCountBeforeSync,
      );
      expect(screen.queryByTestId("dynamic-qris-card")).toBeNull();
    });
  });

  it("never renders a QR card for cash", async () => {
    mockGetTransaction.mockResolvedValue(
      transaction({ paymentMethod: "cash", qrisPayloadHash: null }),
    );
    const screen = render(<TransactionDetailScreen />);

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Pembayaran berhasil" }),
      ).toBeTruthy();
    });

    expect(screen.queryByTestId("dynamic-qris-card")).toBeNull();
    expect(mockDynamicQrisCard).not.toHaveBeenCalled();
  });
});
