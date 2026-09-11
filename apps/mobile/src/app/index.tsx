import { Redirect } from "expo-router";

import { useAuth } from "@/auth/AuthProvider";

export default function IndexRoute() {
  const { booting, session, terminalEnrolled, scopeLocked } = useAuth();
  if (booting) return null;
  if (!session) return <Redirect href="/(auth)/login" />;
  if (session.user.mustChangePassword) {
    return <Redirect href="/(auth)/change-password" />;
  }
  if (scopeLocked || session.contextKind === "account")
    return <Redirect href="/contexts" />;
  if (session.contextKind === "platform") return <Redirect href="/platform" />;
  if (!terminalEnrolled) {
    return <Redirect href="/(auth)/terminal-enrollment" />;
  }
  return <Redirect href="/(app)/(tabs)/home" />;
}
