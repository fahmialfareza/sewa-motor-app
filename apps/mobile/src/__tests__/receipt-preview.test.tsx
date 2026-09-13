import { act, fireEvent, render, waitFor } from "@testing-library/react-native";
import type { ReactNode } from "react";

import PrintTransactionScreen from "@/app/(app)/transactions/[id]/print";
import type { Session, Transaction } from "@/domain/types";
import { formatReceipt } from "@/printer/receipt";
import { receiptFromTransaction, type ReceiptDocument } from "@/printer/types";
import type { PrinterConfig } from "@/security/secure-store";

const mockSession: Session = {
  token: "token",
  sessionId: "SESSION-1",
  dataMode: "sandbox",
  dataSpaceId: "sandbox-space",
  sandboxGeneration: 1,
  establishedAt: "2026-07-30T01:00:00.000Z",
  user: {
    id: "USER-1",
    fullName: "Admin",
    username: "admin",
    role: "admin",
    active: true,
    mustChangePassword: false,
  },
};
const transaction: Transaction = {
  id: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
  revision: 3,
  occurredAt: "2026-07-30T01:00:00.000Z",
  subtotal: 70_000,
  total: 70_000,
  paymentAmount: 70_000,
  originActorId: "USER-1",
  originActorName: "Admin",
  updatedActorName: "Admin",
  terminalId: "TERMINAL-1",
  syncState: "synced",
  printState: "pending",
  paymentMethod: "qris",
  paymentStatus: "success",
  paymentConfirmedRevision: 3,
  qrisPayloadHash: "hash",
  deletedAt: null,
  items: [
    {
      id: "ITEM-1",
      packageId: "PACKAGE-1",
      packageRevision: 2,
      name: "Paket Harian",
      description: "",
      accent: "standard",
      unitPrice: 70_000,
      quantity: 1,
      lineTotal: 70_000,
    },
  ],
};
let mockTransaction = transaction;
let mockConfig: PrinterConfig;
const mockGetTransaction = jest.fn();
const mockBeginAttempt = jest.fn();
const mockCompleteAttempt = jest.fn();
const mockConnect = jest.fn();
const mockPrint = jest.fn();
const mockDisconnect = jest.fn();
const mockRelease = jest.fn();
const mockBeginMutation = jest.fn();
const mockReplace = jest.fn();
const mockSync = { refresh: jest.fn(), syncNow: jest.fn() };

jest.mock("expo-router", () => ({
  useLocalSearchParams: () => ({ id: "01ARZ3NDEKTSV4RRFFQ69G5FAV" }),
  useRouter: () => ({ replace: mockReplace }),
}));
jest.mock("@/auth/AuthProvider", () => ({
  useAuth: () => ({ session: mockSession }),
}));
jest.mock("@/sync/SyncProvider", () => ({ useSyncRuntime: () => mockSync }));
jest.mock("@/db/repositories", () => ({
  getTransaction: (...args: unknown[]) => mockGetTransaction(...args),
  beginPrintAttempt: (...args: unknown[]) => mockBeginAttempt(...args),
  completePrintAttempt: (...args: unknown[]) => mockCompleteAttempt(...args),
}));
jest.mock("@/security/secure-store", () => ({
  readPrinterConfig: async () => mockConfig,
}));
jest.mock("@/printer/service", () => ({
  getConfiguredPrinter: async () => ({
    config: mockConfig,
    printer: {
      connect: mockConnect,
      print: mockPrint,
      disconnect: mockDisconnect,
    },
  }),
}));
jest.mock("@/mode/mutation-barrier", () => ({
  beginLocalMutation: (...args: unknown[]) => mockBeginMutation(...args),
}));
jest.mock("@/mode/mode-store", () => ({
  useModeStore: { getState: () => ({ accessBlocked: false }) },
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
  return { PageHeader: ({ title }: { title: string }) => <Text>{title}</Text> };
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
jest.mock("@/components/ui/PaymentBadge", () => ({
  PaymentMethodBadge: () => null,
}));
jest.mock("@/components/ui/StateView", () => {
  const { Text } =
    jest.requireActual<typeof import("react-native")>("react-native");
  return { StateView: ({ title }: { title: string }) => <Text>{title}</Text> };
});

beforeEach(() => {
  jest.clearAllMocks();
  mockTransaction = transaction;
  mockConfig = {
    adapter: "simulator",
    address: null,
    displayName: "Simulator",
    paperColumns: 32,
  };
  mockGetTransaction.mockImplementation(async () => mockTransaction);
  mockBeginAttempt.mockResolvedValue("attempt-1");
  mockCompleteAttempt.mockResolvedValue(undefined);
  mockConnect.mockResolvedValue(undefined);
  mockPrint.mockResolvedValue({ status: "success" });
  mockDisconnect.mockResolvedValue(undefined);
  mockBeginMutation.mockReturnValue(mockRelease);
  mockSync.refresh.mockResolvedValue(undefined);
  mockSync.syncNow.mockResolvedValue(undefined);
});

it.each([32, 48] as const)(
  "shows a read-only exact %s-column preview without recording a print attempt",
  async (columns) => {
    mockConfig.paperColumns = columns;
    const screen = render(<PrintTransactionScreen />);
    const output = await screen.findByTestId("receipt-preview-text");
    expect(output.props.children).toBe(
      formatReceipt(
        receiptFromTransaction(transaction, false, "sandbox"),
        columns,
      ),
    );
    expect(screen.getByText(new RegExp(`${columns} kolom`))).toBeTruthy();
    expect(mockGetTransaction).toHaveBeenCalledWith(
      transaction.id,
      mockSession,
    );
    expect(mockBeginAttempt).not.toHaveBeenCalled();
    expect(mockBeginMutation).not.toHaveBeenCalled();
    expect(mockConnect).not.toHaveBeenCalled();
    expect(mockPrint).not.toHaveBeenCalled();
  },
);

it("keeps payment gating for stale confirmations", async () => {
  mockTransaction = { ...transaction, paymentConfirmedRevision: 2 };
  const screen = render(<PrintTransactionScreen />);
  expect(await screen.findByText("Pencetakan terkunci")).toBeTruthy();
  expect(screen.queryByTestId("receipt-preview")).toBeNull();
  expect(mockBeginAttempt).not.toHaveBeenCalled();
});

it("describes simulator success and retains the exact document sent, not an accidental copy", async () => {
  const screen = render(<PrintTransactionScreen />);
  await screen.findByTestId("receipt-preview-text");
  fireEvent.press(screen.getByRole("button", { name: "Cetak struk" }));
  expect(await screen.findByText("Simulasi cetak berhasil")).toBeTruthy();
  expect(screen.queryByText("Struk berhasil dicetak!")).toBeNull();
  const sent = mockPrint.mock.calls[0]?.[0] as ReceiptDocument;
  expect(Object.isFrozen(sent)).toBe(true);
  expect(Object.isFrozen(sent.lines)).toBe(true);
  expect(Object.isFrozen(sent.lines[0])).toBe(true);
  expect(sent.isCopy).toBe(false);
  expect(screen.getByTestId("receipt-preview-text").props.children).toBe(
    formatReceipt(sent, 32),
  );
  await waitFor(() => expect(mockRelease).toHaveBeenCalledTimes(1));
  fireEvent.press(screen.getByRole("button", { name: "Cetak salinan" }));
  await waitFor(() => expect(mockPrint).toHaveBeenCalledTimes(2));
  expect(mockPrint.mock.calls[1]?.[0].isCopy).toBe(true);
  await waitFor(() => expect(mockRelease).toHaveBeenCalledTimes(2));
});

it("holds the print barrier until hardware and attempt recording finish", async () => {
  mockConfig.adapter = "integrated";
  let finishPrint!: (result: { status: "success" }) => void;
  mockPrint.mockReturnValue(
    new Promise((resolve) => {
      finishPrint = resolve;
    }),
  );
  let finishRecording!: () => void;
  mockCompleteAttempt.mockReturnValue(
    new Promise<void>((resolve) => {
      finishRecording = resolve;
    }),
  );
  const screen = render(<PrintTransactionScreen />);
  await screen.findByTestId("receipt-preview-text");
  fireEvent.press(screen.getByRole("button", { name: "Cetak struk" }));
  await waitFor(() => expect(mockPrint).toHaveBeenCalledTimes(1));
  expect(mockRelease).not.toHaveBeenCalled();
  await act(async () => {
    finishPrint({ status: "success" });
  });
  expect(mockCompleteAttempt).toHaveBeenCalledTimes(1);
  expect(mockRelease).not.toHaveBeenCalled();
  await act(async () => {
    finishRecording();
  });
  expect(await screen.findByText("Struk berhasil dicetak!")).toBeTruthy();
  await waitFor(() => expect(mockRelease).toHaveBeenCalledTimes(1));
});

it.each(["failed", "unknown"] as const)(
  "records a %s printer result without reporting success or keeping the barrier held",
  async (status) => {
    mockPrint.mockResolvedValue({ status, message: "Printer terputus." });
    const screen = render(<PrintTransactionScreen />);
    await screen.findByTestId("receipt-preview-text");
    fireEvent.press(screen.getByRole("button", { name: "Cetak struk" }));
    await waitFor(() => expect(mockRelease).toHaveBeenCalledTimes(1));
    expect(mockCompleteAttempt).toHaveBeenCalledWith(
      expect.objectContaining({ result: status, error: "Printer terputus." }),
    );
    expect(mockReplace).toHaveBeenCalledWith({
      pathname: "/transactions/[id]/print-failure",
      params: { id: transaction.id, status, message: "Printer terputus." },
    });
    expect(screen.queryByText("Simulasi cetak berhasil")).toBeNull();
    expect(screen.queryByText("Struk berhasil dicetak!")).toBeNull();
  },
);

it("marks a connection failure and always releases the print barrier", async () => {
  mockConnect.mockRejectedValue(new Error("Printer tidak tersambung."));
  const screen = render(<PrintTransactionScreen />);
  await screen.findByTestId("receipt-preview-text");
  fireEvent.press(screen.getByRole("button", { name: "Cetak struk" }));
  expect(await screen.findByText("Printer tidak tersambung.")).toBeTruthy();
  expect(mockPrint).not.toHaveBeenCalled();
  expect(mockCompleteAttempt).toHaveBeenCalledWith(
    expect.objectContaining({
      result: "failed",
      error: "Printer tidak tersambung.",
    }),
  );
  await waitFor(() => expect(mockRelease).toHaveBeenCalledTimes(1));
});
