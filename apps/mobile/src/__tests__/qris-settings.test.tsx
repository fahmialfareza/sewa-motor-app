import { act, fireEvent, render, waitFor } from "@testing-library/react-native";
import type { ReactNode } from "react";
import { Alert, TextInput } from "react-native";

import QrisSettingsScreen from "@/app/(app)/settings/qris";

const STATIC_QRIS =
  "00020101021126320014ID.CO.TEST.WWW011012345678905204729953033605802ID5910SEWA MOTOR6008DENPASAR6304AA64";

const mockAuthState = {
  role: "superadmin" as "superadmin" | "admin",
};
const mockReadQrisConfig = jest.fn();
const mockWriteQrisConfig = jest.fn();
const mockClearQrisConfig = jest.fn();
const mockGetPendingResult = jest.fn();
const mockRequestCameraPermission = jest.fn();
const mockLaunchCamera = jest.fn();
const mockLaunchImageLibrary = jest.fn();
const mockScanFromURL = jest.fn();

jest.mock("@/auth/AuthProvider", () => ({
  useAuth: () => ({
    session: {
      user: { role: mockAuthState.role },
    },
  }),
}));

jest.mock("@/security/secure-store", () => ({
  readQrisConfig: () => mockReadQrisConfig(),
  writeQrisConfig: (value: unknown) => mockWriteQrisConfig(value),
  clearQrisConfig: () => mockClearQrisConfig(),
}));

jest.mock("expo-image-picker", () => ({
  CameraType: { back: "back" },
  getPendingResultAsync: () => mockGetPendingResult(),
  requestCameraPermissionsAsync: () => mockRequestCameraPermission(),
  launchCameraAsync: (options: unknown) => mockLaunchCamera(options),
  launchImageLibraryAsync: (options: unknown) =>
    mockLaunchImageLibrary(options),
}));

jest.mock("expo-camera", () => ({
  scanFromURLAsync: (uri: string, types: string[]) =>
    mockScanFromURL(uri, types),
}));

jest.mock("react-native-qrcode-svg", () => {
  const { Text, View } =
    jest.requireActual<typeof import("react-native")>("react-native");
  return {
    __esModule: true,
    default: ({ value }: { value: string }) => (
      <View accessibilityLabel="Pratinjau QRIS" testID="qris-preview">
        <Text>{value.slice(0, 6)}</Text>
      </View>
    ),
  };
});

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

jest.mock("@/components/ui/Icon", () => ({
  Icon: () => null,
}));

jest.mock("@/components/ui/StateView", () => {
  const { Text, View } =
    jest.requireActual<typeof import("react-native")>("react-native");
  return {
    StateView: ({ message, title }: { message: string; title: string }) => (
      <View>
        <Text>{title}</Text>
        <Text>{message}</Text>
      </View>
    ),
  };
});

function pickedImage(uri = "file:///qris.jpg") {
  return {
    canceled: false as const,
    assets: [
      {
        uri,
        width: 1200,
        height: 1200,
        fileSize: 300_000,
        type: "image" as const,
      },
    ],
  };
}

function qrResult(payload = STATIC_QRIS) {
  return [{ data: payload }];
}

describe("QRIS settings image flow", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockAuthState.role = "superadmin";
    mockReadQrisConfig.mockResolvedValue(null);
    mockWriteQrisConfig.mockResolvedValue(undefined);
    mockClearQrisConfig.mockResolvedValue(undefined);
    mockGetPendingResult.mockResolvedValue(null);
    mockRequestCameraPermission.mockResolvedValue({
      granted: true,
      canAskAgain: true,
    });
    mockLaunchCamera.mockResolvedValue({ canceled: true, assets: null });
    mockLaunchImageLibrary.mockResolvedValue({
      canceled: true,
      assets: null,
    });
    mockScanFromURL.mockResolvedValue([]);
  });

  it("renders a saved static QRIS as locked with no editable payload field", async () => {
    mockReadQrisConfig.mockResolvedValue({ staticPayload: STATIC_QRIS });
    const screen = render(<QrisSettingsScreen />);

    await waitFor(() => {
      expect(screen.getByText("QRIS STATIS TERKUNCI")).toBeTruthy();
    });
    expect(screen.UNSAFE_queryAllByType(TextInput)).toHaveLength(0);
    expect(screen.getByText("Payload tidak dapat diedit manual.")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Ganti QRIS statis" }),
    ).toBeTruthy();
  });

  it("stages a gallery QRIS for review and writes only after Save", async () => {
    mockLaunchImageLibrary.mockResolvedValue(pickedImage());
    mockScanFromURL.mockResolvedValue(qrResult());
    const screen = render(<QrisSettingsScreen />);

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Pilih gambar QRIS" }),
      ).toBeTruthy();
    });
    fireEvent.press(screen.getByRole("button", { name: "Pilih gambar QRIS" }));

    await waitFor(() => {
      expect(screen.getByText("QRIS STATIS TERBACA")).toBeTruthy();
    });
    expect(mockScanFromURL).toHaveBeenCalledWith("file:///qris.jpg", ["qr"]);
    expect(mockWriteQrisConfig).not.toHaveBeenCalled();

    fireEvent.press(screen.getByRole("button", { name: "Simpan QRIS statis" }));

    await waitFor(() => {
      expect(mockWriteQrisConfig).toHaveBeenCalledWith({
        staticPayload: STATIC_QRIS,
      });
      expect(screen.getByText("QRIS STATIS TERKUNCI")).toBeTruthy();
    });
  });

  it("requires confirmation and preserves the old QRIS when replacement scanning fails", async () => {
    mockReadQrisConfig.mockResolvedValue({ staticPayload: STATIC_QRIS });
    mockLaunchImageLibrary.mockResolvedValue(pickedImage("file:///empty.jpg"));
    const alert = jest.spyOn(Alert, "alert");
    const screen = render(<QrisSettingsScreen />);

    await waitFor(() => {
      expect(screen.getByText("QRIS STATIS TERKUNCI")).toBeTruthy();
    });
    fireEvent.press(screen.getByRole("button", { name: "Ganti QRIS statis" }));

    const buttons = alert.mock.calls[0]?.[2];
    const proceed = buttons?.find((button) => button.text === "Lanjutkan");
    await act(async () => {
      proceed?.onPress?.();
    });
    fireEvent.press(screen.getByRole("button", { name: "Pilih gambar QRIS" }));

    await waitFor(() => {
      expect(screen.getByText(/Kode QR tidak ditemukan/)).toBeTruthy();
    });
    expect(mockWriteQrisConfig).not.toHaveBeenCalled();
    expect(mockClearQrisConfig).not.toHaveBeenCalled();

    fireEvent.press(screen.getByRole("button", { name: "Batal mengganti" }));
    expect(screen.getByText("QRIS STATIS TERKUNCI")).toBeTruthy();
    alert.mockRestore();
  });

  it("does not open the camera when permission is denied", async () => {
    mockRequestCameraPermission.mockResolvedValue({
      granted: false,
      canAskAgain: false,
    });
    const screen = render(<QrisSettingsScreen />);

    await waitFor(() => {
      expect(
        screen.getByRole("button", { name: "Ambil foto QRIS" }),
      ).toBeTruthy();
    });
    fireEvent.press(screen.getByRole("button", { name: "Ambil foto QRIS" }));

    await waitFor(() => {
      expect(screen.getByText(/Aktifkan izin Kamera/)).toBeTruthy();
    });
    expect(mockLaunchCamera).not.toHaveBeenCalled();
  });

  it("recovers an Android pending image as an unsaved candidate", async () => {
    mockGetPendingResult.mockResolvedValue(pickedImage("file:///pending.jpg"));
    mockScanFromURL.mockResolvedValue(qrResult());
    const screen = render(<QrisSettingsScreen />);

    await waitFor(() => {
      expect(screen.getByText("QRIS STATIS TERBACA")).toBeTruthy();
    });
    expect(mockScanFromURL).toHaveBeenCalledWith("file:///pending.jpg", ["qr"]);
    expect(mockWriteQrisConfig).not.toHaveBeenCalled();
  });

  it("does not read QRIS configuration for a non-superadmin", async () => {
    mockAuthState.role = "admin";
    const screen = render(<QrisSettingsScreen />);

    expect(screen.getByText("Akses dibatasi")).toBeTruthy();
    await act(async () => undefined);
    expect(mockReadQrisConfig).not.toHaveBeenCalled();
  });
});
