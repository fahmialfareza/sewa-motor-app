import type { LoginResponse } from "@/api/contracts";
import type { Session } from "@/domain/types";

export function sessionFromLoginResponse(result: LoginResponse): Session {
  const dataMode = result.dataMode === "sandbox" ? "sandbox" : "production";
  return {
    token: result.sessionToken,
    sessionId: result.sessionId,
    user: result.user,
    establishedAt: new Date().toISOString(),
    dataMode,
    dataSpaceId: result.dataSpaceId,
    sandboxGeneration:
      dataMode === "sandbox" && result.sandboxGeneration > 0
        ? result.sandboxGeneration
        : null,
  };
}
