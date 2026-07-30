import { fireEvent, render, waitFor } from "@testing-library/react-native";
import type { ReactNode } from "react";

import UsersScreen from "@/app/(app)/(tabs)/users";
import EditUserScreen from "@/app/(app)/users/[id]/edit";
import type { UserSummary } from "@/domain/types";
import { SERVER_UNREACHABLE_MESSAGE } from "@/utils/errors";

const mockApiRequest = jest.fn();
const mockRouterPush = jest.fn();
const mockRouterBack = jest.fn();

const sessionUser: UserSummary = {
  id: "USER-1",
  fullName: "Super Admin",
  username: "superadmin",
  role: "superadmin",
  active: true,
  mustChangePassword: false,
};

const loadedUser: UserSummary = {
  id: "USER-2",
  fullName: "Admin Toko",
  username: "admin.toko",
  role: "admin",
  active: true,
  mustChangePassword: false,
};

jest.mock("@/api/client", () => ({
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}));

jest.mock("@/auth/AuthProvider", () => ({
  useAuth: () => ({
    session: {
      token: "session-token",
      user: sessionUser,
    },
  }),
}));

jest.mock("expo-router", () => {
  const React = jest.requireActual<typeof import("react")>("react");
  return {
    useRouter: () => ({ push: mockRouterPush, back: mockRouterBack }),
    useLocalSearchParams: () => ({ id: loadedUser.id }),
    useFocusEffect: (effect: () => void | (() => void)) =>
      React.useEffect(effect, [effect]),
  };
});

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

jest.mock("@/components/ui/Card", () => {
  const { View } =
    jest.requireActual<typeof import("react-native")>("react-native");
  return {
    Card: ({ children }: { children: ReactNode }) => <View>{children}</View>,
  };
});

jest.mock("@/components/ui/Field", () => {
  const { TextInput } =
    jest.requireActual<typeof import("react-native")>("react-native");
  return {
    Field: ({ label }: { label: string }) => (
      <TextInput accessibilityLabel={label} />
    ),
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
      onPress?: () => void;
    }) => (
      <Pressable accessibilityRole="button" onPress={onPress}>
        <Text>{children}</Text>
      </Pressable>
    ),
  };
});

jest.mock("@/components/ui/StateView", () => {
  const { Pressable, Text, View } =
    jest.requireActual<typeof import("react-native")>("react-native");
  return {
    StateView: ({
      actionLabel,
      message,
      onAction,
      title,
    }: {
      actionLabel?: string;
      message: string;
      onAction?: () => void;
      title: string;
    }) => (
      <View>
        <Text>{title}</Text>
        <Text>{message}</Text>
        {actionLabel && onAction ? (
          <Pressable accessibilityRole="button" onPress={onAction}>
            <Text>{actionLabel}</Text>
          </Pressable>
        ) : null}
      </View>
    ),
  };
});

jest.mock("@/components/forms/UserForm", () => {
  const { Text } =
    jest.requireActual<typeof import("react-native")>("react-native");
  return {
    UserForm: ({ initial }: { initial: UserSummary }) => (
      <Text>{initial.fullName}</Text>
    ),
  };
});

const nativeConnectionError = new Error(
  "fetch failed: java.net.ConnectException: Failed to connect to /192.168.18.254:8080",
);

describe("user screen connection errors", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("shows an understandable list error and retries", async () => {
    mockApiRequest
      .mockRejectedValueOnce(nativeConnectionError)
      .mockResolvedValueOnce([loadedUser]);
    const screen = render(<UsersScreen />);

    await waitFor(() => {
      expect(screen.getByText("Pengguna belum dapat dimuat")).toBeTruthy();
      expect(screen.getByText(SERVER_UNREACHABLE_MESSAGE)).toBeTruthy();
    });
    expect(screen.queryByText(nativeConnectionError.message)).toBeNull();

    fireEvent.press(screen.getByRole("button", { name: "Coba lagi" }));

    await waitFor(() => {
      expect(screen.getByText(loadedUser.fullName)).toBeTruthy();
      expect(screen.queryByText("Pengguna belum dapat dimuat")).toBeNull();
    });
  });

  it("shows an understandable edit error and retries", async () => {
    mockApiRequest
      .mockRejectedValueOnce(nativeConnectionError)
      .mockResolvedValueOnce(loadedUser);
    const screen = render(<EditUserScreen />);

    await waitFor(() => {
      expect(screen.getByText("Pengguna belum dapat dimuat")).toBeTruthy();
      expect(screen.getByText(SERVER_UNREACHABLE_MESSAGE)).toBeTruthy();
    });
    expect(screen.queryByText(nativeConnectionError.message)).toBeNull();

    fireEvent.press(screen.getByRole("button", { name: "Coba lagi" }));

    await waitFor(() => {
      expect(screen.getByText(loadedUser.fullName)).toBeTruthy();
      expect(screen.queryByText("Pengguna belum dapat dimuat")).toBeNull();
    });
  });
});
