import { useEffect, type ReactNode } from "react";
import { useShallow } from "zustand/react/shallow";

import { useAuthStore, type AuthStore } from "./auth-store";

type AuthRuntime = Omit<AuthStore, "hydrate">;

const selectAuthRuntime = (state: AuthStore): AuthRuntime => ({
  session: state.session,
  booting: state.booting,
  demoEnabled: state.demoEnabled,
  terminalEnrolled: state.terminalEnrolled,
  login: state.login,
  demoLogin: state.demoLogin,
  changePassword: state.changePassword,
  enrollTerminal: state.enrollTerminal,
  logout: state.logout,
});

export function AuthProvider({ children }: { children: ReactNode }) {
  const hydrate = useAuthStore((state) => state.hydrate);

  useEffect(() => {
    void hydrate().catch(() => undefined);
  }, [hydrate]);

  return <>{children}</>;
}

export function useAuth(): AuthRuntime {
  return useAuthStore(useShallow(selectAuthRuntime));
}
