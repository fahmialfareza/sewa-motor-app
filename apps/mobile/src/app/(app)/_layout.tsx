import { Redirect, Stack } from "expo-router";

import { useAuth } from "@/auth/AuthProvider";

export default function ProtectedLayout() {
  const { booting, session, terminalEnrolled } = useAuth();
  if (booting) return null;
  if (!session) return <Redirect href="/(auth)/login" />;
  if (session.user.mustChangePassword) {
    return <Redirect href="/(auth)/change-password" />;
  }
  if (!terminalEnrolled) {
    return <Redirect href="/(auth)/terminal-enrollment" />;
  }
  return <Stack screenOptions={{ headerShown: false }} />;
}
