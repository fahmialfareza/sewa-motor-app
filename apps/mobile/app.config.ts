import type { ConfigContext, ExpoConfig } from "expo/config";

const DEFAULT_ANDROID_PACKAGE = "com.example.sewamotorpos";

export default ({ config }: ConfigContext): ExpoConfig => ({
  ...config,
  name: "Sewa Motor POS",
  slug: "sewa-motor-pos",
  version: "0.1.0",
  orientation: "portrait",
  userInterfaceStyle: "light",
  scheme: "sewamotor",
  newArchEnabled: true,
  android: {
    package: process.env.ANDROID_APPLICATION_ID ?? DEFAULT_ANDROID_PACKAGE,
    adaptiveIcon: {
      backgroundColor: "#003D9B",
    },
    permissions: [
      "android.permission.BLUETOOTH",
      "android.permission.BLUETOOTH_ADMIN",
      "android.permission.BLUETOOTH_CONNECT",
      "android.permission.BLUETOOTH_SCAN",
      "android.permission.ACCESS_FINE_LOCATION",
    ],
    blockedPermissions: [
      "android.permission.READ_MEDIA_IMAGES",
      "android.permission.READ_MEDIA_VIDEO",
    ],
  },
  plugins: [
    "expo-router",
    [
      "expo-sqlite",
      {
        useSQLCipher: true,
        enableFTS: true,
      },
    ],
    [
      "expo-secure-store",
      {
        configureAndroidBackup: true,
        faceIDPermission: "Izinkan Sewa Motor POS mengakses kredensial aman.",
      },
    ],
    "expo-background-task",
  ],
  experiments: {
    typedRoutes: false,
  },
  extra: {
    apiUrl:
      process.env.EXPO_PUBLIC_API_BASE_URL ??
      process.env.EXPO_PUBLIC_API_URL ??
      "http://10.0.2.2:8080/api/v1",
    enableDemoLogin: process.env.EXPO_PUBLIC_ENABLE_DEMO_LOGIN === "true",
    eas: {
      projectId: process.env.EAS_PROJECT_ID,
    },
  },
  runtimeVersion: {
    policy: "appVersion",
  },
  updates: {
    enabled: false,
  },
});
