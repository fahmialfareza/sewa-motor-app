import { Redirect, Stack } from "expo-router";
import { useAuth } from "@/auth/AuthProvider";

export default function ManagementLayout() {
  const { session, booting, bootError, scopeLocked } = useAuth();
  if (booting) return null;
  if (!session) return <Redirect href="/(auth)/login" />;
  if (session.user.mustChangePassword)
    return <Redirect href="/(auth)/change-password" />;
  if (
    bootError ||
    scopeLocked ||
    session.contextKind !== "account" ||
    session.user.role !== "superadmin"
  )
    return <Redirect href="/contexts" />;
  return (
    <Stack key={session.sessionId} screenOptions={{ headerShown: false }} />
  );
}
