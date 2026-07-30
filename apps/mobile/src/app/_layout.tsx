import { JetBrainsMono_500Medium } from "@expo-google-fonts/jetbrains-mono/500Medium";
import { Roboto_400Regular } from "@expo-google-fonts/roboto/400Regular";
import { Roboto_500Medium } from "@expo-google-fonts/roboto/500Medium";
import { Roboto_600SemiBold } from "@expo-google-fonts/roboto/600SemiBold";
import { Roboto_700Bold } from "@expo-google-fonts/roboto/700Bold";
import { Roboto_800ExtraBold } from "@expo-google-fonts/roboto/800ExtraBold";
import { useFonts } from "expo-font";
import { Stack } from "expo-router";
import * as SplashScreen from "expo-splash-screen";
import { StatusBar } from "expo-status-bar";
import { useEffect, useState } from "react";
import { StyleSheet, Text, View } from "react-native";
import { KeyboardProvider } from "react-native-keyboard-controller";
import { SafeAreaProvider } from "react-native-safe-area-context";

import { AuthProvider } from "@/auth/AuthProvider";
import { initializeDatabase } from "@/db/client";
import "@/sync/background";
import { SyncProvider } from "@/sync/SyncProvider";
import { colors, spacing, textStyles } from "@/theme/tokens";

void SplashScreen.preventAutoHideAsync();

export default function RootLayout() {
  const [fontsLoaded, fontError] = useFonts({
    Roboto_400Regular,
    Roboto_500Medium,
    Roboto_600SemiBold,
    Roboto_700Bold,
    Roboto_800ExtraBold,
    JetBrainsMono_500Medium,
  });
  const [databaseReady, setDatabaseReady] = useState(false);
  const [databaseError, setDatabaseError] = useState<Error | null>(null);

  useEffect(() => {
    void initializeDatabase()
      .then(() => setDatabaseReady(true))
      .catch((error: unknown) =>
        setDatabaseError(
          error instanceof Error ? error : new Error("Database gagal dibuka."),
        ),
      );
  }, []);

  useEffect(() => {
    if ((fontsLoaded || fontError) && (databaseReady || databaseError)) {
      void SplashScreen.hideAsync();
    }
  }, [databaseError, databaseReady, fontError, fontsLoaded]);

  if (databaseError) {
    return (
      <View style={styles.failure}>
        <Text style={textStyles.title}>Database tidak dapat dibuka</Text>
        <Text style={styles.failureMessage}>
          Kredensial enkripsi lokal tidak cocok atau penyimpanan perangkat
          bermasalah. Jangan hapus data sebelum menghubungi dukungan.
        </Text>
        <Text style={styles.code}>{databaseError.message}</Text>
      </View>
    );
  }
  if ((!fontsLoaded && !fontError) || !databaseReady) return null;

  return (
    <SafeAreaProvider>
      <KeyboardProvider>
        <AuthProvider>
          <SyncProvider>
            <StatusBar style="light" />
            <Stack screenOptions={{ headerShown: false }}>
              <Stack.Screen name="index" />
              <Stack.Screen name="(auth)" />
              <Stack.Screen name="(app)" />
            </Stack>
          </SyncProvider>
        </AuthProvider>
      </KeyboardProvider>
    </SafeAreaProvider>
  );
}

const styles = StyleSheet.create({
  failure: {
    flex: 1,
    padding: spacing.lg,
    backgroundColor: colors.surface,
    justifyContent: "center",
    gap: spacing.md,
  },
  failureMessage: { ...textStyles.body, color: colors.textMuted },
  code: { ...textStyles.technical, color: colors.error },
});
