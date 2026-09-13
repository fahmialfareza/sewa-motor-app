import type { LoginResponse } from "@/api/contracts";
import { useAuthStore } from "@/auth/auth-store";
import { registerAccessFailureHandler } from "@/api/client";
import {
  INITIAL_TENANT_ID,
  PRODUCTION_DATA_SPACE_ID,
  type Session,
} from "@/domain/types";
import { setModeFromSession, useModeStore } from "@/mode/mode-store";
import {
  beginLocalMutation,
  resetMutationBarrierForTests,
} from "@/mode/mutation-barrier";

const mockNetwork = jest.fn();
const mockApiRequest = jest.fn();
const mockRunSync = jest.fn();
const mockPending = jest.fn();
const mockWriteSession = jest.fn();
const mockPrepare = jest.fn();
const mockMarkScopeRevalidated = jest.fn();
const mockMarkEnrolled = jest.fn();
const mockClearSession = jest.fn();
const mockGetIdentity = jest.fn();
const mockQuarantine = jest.fn();
jest.mock("@react-native-community/netinfo", () => ({
  __esModule: true,
  default: { fetch: () => mockNetwork() },
}));
jest.mock("@/api/client", () => ({
  registerAccessFailureHandler: jest.fn(),
  apiRequest: (...args: unknown[]) => mockApiRequest(...args),
}));
jest.mock("@/db/client", () => ({
  prepareDatabaseForSession: (...args: unknown[]) => mockPrepare(...args),
}));
jest.mock("@/db/repositories", () => ({
  countPendingOutbox: (...args: unknown[]) => mockPending(...args),
  recoverInterruptedPrintAttempts: jest.fn(),
}));
jest.mock("@/security/secure-store", () => ({
  getOrCreateInstallationId: async () => "physical-phone",
  writeSession: (...args: unknown[]) => mockWriteSession(...args),
  clearSession: () => mockClearSession(),
  clearAuthNotice: jest.fn(),
  readSession: jest.fn(),
  readAuthNotice: jest.fn(),
}));
jest.mock("@/security/terminal-identity", () => ({
  getOrCreateTerminalIdentity: (...args: unknown[]) => mockGetIdentity(...args),
  markTerminalEnrolled: (...args: unknown[]) => mockMarkEnrolled(...args),
  markTerminalRevoked: jest.fn(),
}));
jest.mock("@/sync/engine", () => ({
  runSync: (...args: unknown[]) => mockRunSync(...args),
}));
jest.mock("@/sync/state-handoff", () => ({
  hydrateSyncStateForSession: async () => undefined,
  resetSyncStateForSession: jest.fn(),
}));
jest.mock("@/tenant/quarantine", () => ({
  SCOPE_ACCESS_CODES: new Set(["ACCOUNT_ACCESS_CHANGED"]),
  markScopeRevalidated: (...args: unknown[]) =>
    mockMarkScopeRevalidated(...args),
  blockedScopeReason: async () => null,
  quarantineScope: (...args: unknown[]) => mockQuarantine(...args),
}));
const handleAccessFailure = jest.mocked(registerAccessFailureHandler).mock
  .calls[0]![0];

const original: Session = {
  token: "old-token",
  sessionId: "old-session",
  contextKind: "tenant",
  tenantId: INITIAL_TENANT_ID,
  dataMode: "sandbox",
  dataSpaceId: "00000000-0000-4000-8000-000000000109",
  sandboxGeneration: 9,
  establishedAt: "2026-09-11T00:00:00Z",
  user: {
    id: "staff-a",
    fullName: "Staff A",
    username: "staff",
    role: "superadmin",
    active: true,
    mustChangePassword: false,
  },
};
const targetTenantId = "00000000-0000-4000-8000-000000000201";
const tenantResponse = {
  sessionToken: "next-token",
  sessionId: "next-session",
  contextKind: "tenant",
  tenantId: targetTenantId,
  membershipId: "membership-b",
  isPlatformAdmin: false,
  tenant: {
    id: targetTenantId,
    name: "Bisnis B",
    slug: "bisnis-b",
    status: "active",
  },
  dataMode: "production",
  dataSpaceId: "00000000-0000-4000-8000-000000000301",
  sandboxGeneration: 0,
  user: { ...original.user, role: "admin" },
  terminal: { id: "enrollment-b" },
} as LoginResponse;
const accountResponse = {
  ...tenantResponse,
  sessionToken: "account-token",
  sessionId: "account-session",
  contextKind: "account",
  tenantId: null,
  membershipId: null,
  tenant: null,
  dataMode: null,
  dataSpaceId: null,
  terminal: null,
} as LoginResponse;

describe("authenticated tenant context handoff", () => {
  beforeEach(() => {
    jest.resetAllMocks();
    resetMutationBarrierForTests();
    setModeFromSession(original);
    useAuthStore.setState({
      session: original,
      terminalEnrolled: true,
      scopeLocked: false,
      switchingMode: false,
      booting: false,
      bootError: null,
      notice: null,
    });
    mockNetwork.mockResolvedValue({
      isConnected: true,
      isInternetReachable: true,
    });
    mockApiRequest.mockResolvedValue(tenantResponse);
    mockPending.mockResolvedValue(0);
    mockGetIdentity.mockResolvedValue({ serverTerminalId: null });
    mockRunSync.mockResolvedValue({
      pushed: 0,
      pulled: 0,
      conflicts: 0,
      completedAt: "2026-09-11T00:00:00Z",
    });
  });
  afterEach(() => {
    resetMutationBarrierForTests();
    setModeFromSession(null);
  });

  it("holds the handoff until the entire physical-print lease finishes, then drains and enters target Production", async () => {
    const finishPrint = beginLocalMutation(original);
    const switching = useAuthStore
      .getState()
      .switchContext("tenant", targetTenantId);
    expect(useAuthStore.getState().switchingMode).toBe(true);
    await Promise.resolve();
    expect(mockRunSync).not.toHaveBeenCalled();
    expect(mockApiRequest).not.toHaveBeenCalled();
    expect(() => beginLocalMutation(original)).toThrow("Pergantian mode");
    finishPrint();
    await switching;
    expect(mockRunSync).toHaveBeenNthCalledWith(1, original);
    expect(mockPending).toHaveBeenCalledWith(original);
    expect(mockApiRequest).toHaveBeenCalledWith("/auth/switch-context", {
      method: "POST",
      token: original.token,
      body: {
        kind: "tenant",
        tenantId: targetTenantId,
        installationId: "physical-phone",
      },
    });
    const next = useAuthStore.getState().session!;
    expect(next).toMatchObject({
      tenantId: targetTenantId,
      dataMode: "production",
      sandboxGeneration: null,
      membershipId: "membership-b",
      user: { role: "admin" },
    });
    expect(mockWriteSession).toHaveBeenCalledWith(next);
    expect(mockPrepare).toHaveBeenCalledWith(next);
    expect(mockMarkEnrolled).toHaveBeenCalledWith(
      "enrollment-b",
      targetTenantId,
    );
    expect(mockRunSync).toHaveBeenNthCalledWith(2, next);
    expect(mockWriteSession.mock.invocationCallOrder[0]).toBeLessThan(
      mockPrepare.mock.invocationCallOrder[0]!,
    );
    expect(useModeStore.getState()).toMatchObject({
      tenantId: targetTenantId,
      contextKind: "tenant",
      dataMode: "production",
    });
    expect(useAuthStore.getState().switchingMode).toBe(false);
  });

  it.each(["offline", "pending", "failed-sync"])(
    "keeps original credentials and queues on %s",
    async (failure) => {
      if (failure === "offline")
        mockNetwork.mockResolvedValue({ isConnected: false });
      if (failure === "pending") mockPending.mockResolvedValue(2);
      if (failure === "failed-sync")
        mockRunSync.mockRejectedValue(new Error("server offline"));
      await expect(
        useAuthStore.getState().switchContext("tenant", targetTenantId),
      ).rejects.toThrow();
      expect(useAuthStore.getState().session).toBe(original);
      expect(useAuthStore.getState().switchingMode).toBe(false);
      expect(mockApiRequest).not.toHaveBeenCalled();
      expect(mockWriteSession).not.toHaveBeenCalled();
    },
  );

  it("can leave a locked tenant without replaying quarantined work", async () => {
    useAuthStore.setState({ scopeLocked: true });
    useModeStore.setState({ accessBlocked: true });
    await useAuthStore.getState().switchContext("tenant", targetTenantId);
    expect(mockPending).not.toHaveBeenCalled();
    expect(mockRunSync).toHaveBeenCalledTimes(1);
    expect(mockRunSync).toHaveBeenCalledWith(
      expect.objectContaining({ tenantId: targetTenantId }),
    );
    expect(mockMarkScopeRevalidated).toHaveBeenCalledWith(
      expect.objectContaining({ tenantId: targetTenantId }),
    );
    expect(useAuthStore.getState().scopeLocked).toBe(false);
  });

  it("does not enroll or sync a business when exchanging into account context", async () => {
    mockApiRequest.mockResolvedValue(accountResponse);
    await useAuthStore.getState().switchContext("account");
    expect(useAuthStore.getState().session).toMatchObject({
      contextKind: "account",
      tenantId: null,
      dataSpaceId: null,
    });
    expect(mockRunSync).toHaveBeenCalledTimes(1);
    expect(mockMarkEnrolled).not.toHaveBeenCalled();
    expect(mockGetIdentity).not.toHaveBeenCalled();
    expect(mockMarkScopeRevalidated).not.toHaveBeenCalled();
  });

  it.each(["secure-store", "database"])(
    "hides the revoked old scope when replacement %s setup fails",
    async (failure) => {
      if (failure === "secure-store")
        mockWriteSession.mockRejectedValue(new Error("keystore unavailable"));
      else mockPrepare.mockRejectedValue(new Error("database unavailable"));
      await expect(
        useAuthStore.getState().switchContext("tenant", targetTenantId),
      ).rejects.toThrow();
      expect(useAuthStore.getState().session?.sessionId).toBe("next-session");
      expect(useAuthStore.getState().bootError).toBeTruthy();
      expect(useModeStore.getState().tenantId).toBe(targetTenantId);
      expect(useAuthStore.getState().switchingMode).toBe(false);
    },
  );

  it("retains the replacement session if its initial pull fails", async () => {
    mockRunSync
      .mockResolvedValueOnce({})
      .mockRejectedValueOnce(new Error("connection lost"));
    await expect(
      useAuthStore.getState().switchContext("tenant", targetTenantId),
    ).rejects.toThrow("connection lost");
    expect(useAuthStore.getState()).toMatchObject({
      session: { sessionId: "next-session" },
      terminalEnrolled: true,
      switchingMode: false,
      bootError: null,
    });
  });

  it("keeps the account chooser for multiple memberships and after password recovery", async () => {
    mockApiRequest
      .mockResolvedValueOnce(accountResponse)
      .mockResolvedValueOnce({
        tenants: [
          { tenant: { id: INITIAL_TENANT_ID, status: "active" } },
          { tenant: { id: targetTenantId, status: "active" } },
        ],
        platformAdmin: false,
      });
    await useAuthStore.getState().login("staff", "test-password");
    expect(useAuthStore.getState().session?.contextKind).toBe("account");
    expect(mockApiRequest).not.toHaveBeenCalledWith(
      "/auth/switch-context",
      expect.anything(),
    );
    mockApiRequest.mockClear();
    mockApiRequest.mockResolvedValueOnce({
      ...accountResponse,
      user: { ...original.user, mustChangePassword: true },
    });
    await useAuthStore.getState().login("staff", "temporary-password");
    expect(mockApiRequest).toHaveBeenCalledTimes(1);
    expect(useAuthStore.getState().session?.user.mustChangePassword).toBe(true);
  });

  it("automatically selects the sole active membership in Production on normal login", async () => {
    mockApiRequest
      .mockResolvedValueOnce(accountResponse)
      .mockResolvedValueOnce({
        tenants: [{ tenant: { id: targetTenantId, status: "active" } }],
        platformAdmin: false,
      })
      .mockResolvedValueOnce(tenantResponse);
    await useAuthStore.getState().login("staff", "test-password");
    expect(useAuthStore.getState().session).toMatchObject({
      tenantId: targetTenantId,
      dataMode: "production",
    });
    expect(mockPending).not.toHaveBeenCalled();
  });

  it("logs out without deleting business storage and removes the selected context", async () => {
    useAuthStore.setState({
      session: {
        ...original,
        dataMode: "production",
        dataSpaceId: PRODUCTION_DATA_SPACE_ID,
        sandboxGeneration: null,
      },
    });
    await useAuthStore.getState().logout();
    expect(mockClearSession).toHaveBeenCalledTimes(1);
    expect(mockPrepare).not.toHaveBeenCalled();
    expect(useAuthStore.getState().session).toBeNull();
    expect(useModeStore.getState()).toMatchObject({
      contextKind: "account",
      tenantId: null,
      dataMode: "production",
    });
  });

  it("upgrades only after printing completes and drains the unchanged legacy queue before issuing protocol3", async () => {
    const upgraded = {
      ...tenantResponse,
      tenantId: original.tenantId,
      dataMode: "sandbox",
      dataSpaceId: original.dataSpaceId,
      sandboxGeneration: 9,
      protocolVersion: 3,
      sandboxQrisPolicy: "transaction_total",
    } as LoginResponse;
    mockApiRequest.mockResolvedValue(upgraded);
    const finishPrint = beginLocalMutation(original);
    const promise = useAuthStore.getState().upgradeSession();
    await Promise.resolve();
    expect(mockRunSync).not.toHaveBeenCalled();
    finishPrint();
    await promise;
    expect(mockRunSync).toHaveBeenNthCalledWith(1, original);
    expect(mockPending).toHaveBeenCalledWith(original);
    expect(mockApiRequest).toHaveBeenCalledWith("/auth/upgrade-session", {
      method: "POST",
      token: original.token,
      body: { protocolVersion: 3 },
    });
    expect(useAuthStore.getState().session).toMatchObject({
      dataMode: "sandbox",
      dataSpaceId: original.dataSpaceId,
      sandboxGeneration: 9,
      protocolVersion: 3,
      sandboxQrisPolicy: "transaction_total",
    });
    expect(mockRunSync.mock.invocationCallOrder[0]).toBeLessThan(
      mockApiRequest.mock.invocationCallOrder[0]!,
    );
    expect(original.sandboxQrisPolicy).toBeUndefined();
  });

  it.each(["offline", "pending", "failed-sync", "failed-exchange"])(
    "keeps legacy auth and queue evidence after %s during upgrade",
    async (failure) => {
      if (failure === "offline")
        mockNetwork.mockResolvedValue({ isConnected: false });
      if (failure === "pending") mockPending.mockResolvedValue(1);
      if (failure === "failed-sync")
        mockRunSync.mockRejectedValue(new Error("sync unavailable"));
      if (failure === "failed-exchange")
        mockApiRequest.mockRejectedValue(new Error("upgrade unavailable"));
      await expect(useAuthStore.getState().upgradeSession()).rejects.toThrow();
      expect(useAuthStore.getState().session).toBe(original);
      expect(useAuthStore.getState().switchingMode).toBe(false);
      expect(mockWriteSession).not.toHaveBeenCalled();
      expect(mockClearSession).not.toHaveBeenCalled();
    },
  );

  it("quarantines account-revoked work and requires fresh login without deleting databases or signing keys", async () => {
    await handleAccessFailure(original.token, "ACCOUNT_ACCESS_CHANGED");
    expect(mockQuarantine).toHaveBeenCalledWith(
      original,
      "ACCOUNT_ACCESS_CHANGED",
    );
    expect(mockClearSession).toHaveBeenCalledTimes(1);
    expect(useAuthStore.getState().session).toBeNull();
    expect(useAuthStore.getState().notice).toContain("Masuk kembali");
    expect(mockRunSync).not.toHaveBeenCalled();
    expect(mockPrepare).not.toHaveBeenCalled();
    expect(mockMarkScopeRevalidated).not.toHaveBeenCalled();
  });

  it("does not clear a newer login after a delayed account quarantine", async () => {
    let complete!: () => void;
    mockQuarantine.mockImplementationOnce(
      () =>
        new Promise<void>((resolve) => {
          complete = resolve;
        }),
    );
    const failure = handleAccessFailure(
      original.token,
      "ACCOUNT_ACCESS_CHANGED",
    );
    const next = {
      ...original,
      token: "fresh-token",
      sessionId: "fresh-session",
    };
    useAuthStore.setState({ session: next, scopeLocked: false });
    setModeFromSession(next);
    complete();
    await failure;
    expect(mockClearSession).not.toHaveBeenCalled();
    expect(useAuthStore.getState().session).toBe(next);
  });
});
