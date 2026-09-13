import type { Session } from "@/domain/types";
import { runSync } from "@/sync/engine";

const mockNetworkFetch = jest.fn();
const mockApiRequest = jest.fn<Promise<unknown>, unknown[]>();
const mockGetOutboxOperations = jest.fn();
const mockGetSyncMetadata = jest.fn();
const mockApplyRemoteChanges = jest.fn();
const mockMarkOutboxResult = jest.fn();
const mockSetSyncError = jest.fn();
const mockReadSession = jest.fn();
const mockWriteSession = jest.fn();
const mockCacheConfiguration = jest.fn();
const mockNoticeAccess = jest.fn();

jest.mock("@react-native-community/netinfo", () => ({
  __esModule: true,
  default: { fetch: (...args: unknown[]) => mockNetworkFetch(...args) },
}));

jest.mock("@/api/client", () => {
  class ApiError extends Error {
    readonly status: number;
    readonly code: string;
    readonly details: unknown;
    readonly requestId: string | undefined;

    constructor(input: {
      status: number;
      code: string;
      message: string;
      details?: unknown;
      requestId?: string;
    }) {
      super(input.message);
      this.status = input.status;
      this.code = input.code;
      this.details = input.details;
      this.requestId = input.requestId;
    }
  }
  return {
    ApiError,
    apiRequest: (...args: unknown[]) => mockApiRequest(...args),
    noticeAccessFailure: (...args: unknown[]) => mockNoticeAccess(...args),
  };
});

jest.mock("@/db/repositories", () => ({
  applyRemoteChanges: (...args: unknown[]) => mockApplyRemoteChanges(...args),
  getOutboxOperations: (...args: unknown[]) => mockGetOutboxOperations(...args),
  getSyncMetadata: (...args: unknown[]) => mockGetSyncMetadata(...args),
  getTransaction: jest.fn(),
  markOutboxResult: (...args: unknown[]) => mockMarkOutboxResult(...args),
  setSyncError: (...args: unknown[]) => mockSetSyncError(...args),
}));

jest.mock("@/security/secure-store", () => ({
  readTerminalIdentity: jest.fn(),
  readSession: () => mockReadSession(),
  writeSession: (...args: unknown[]) => mockWriteSession(...args),
}));

jest.mock("@/security/terminal-identity", () => ({
  markTerminalEnrolled: jest.fn(),
  markTerminalRevoked: jest.fn(),
}));

const sandboxSession: Session = {
  token: "sandbox-token",
  sessionId: "SANDBOX-SESSION-7",
  establishedAt: "2026-09-05T00:00:00.000Z",
  dataMode: "sandbox",
  dataSpaceId: "00000000-0000-4000-8000-000000000207",
  sandboxGeneration: 7,
  user: {
    id: "USER-1",
    fullName: "Putu",
    username: "putu",
    role: "admin",
    active: true,
    mustChangePassword: false,
  },
};

function pullPage(index: number, last: number) {
  return {
    changes: [
      {
        cursor: `sandbox:7:${index}`,
        aggregate: "audit",
        aggregateId: `AUDIT-${index}`,
        action: "upsert",
        payload: { index },
        changedAt: "2026-09-05T00:00:00.000Z",
      },
    ],
    cursor: `sandbox:7:${index}`,
    hasMore: index < last,
  };
}

describe("Sandbox sync pagination and retirement", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockNetworkFetch.mockResolvedValue({
      isConnected: true,
      isInternetReachable: true,
    });
    mockGetOutboxOperations.mockResolvedValue([]);
    mockGetSyncMetadata.mockResolvedValue({ cursor: null });
    mockApplyRemoteChanges.mockResolvedValue(undefined);
    mockMarkOutboxResult.mockResolvedValue(undefined);
    mockSetSyncError.mockResolvedValue(undefined);
  });

  it("pulls every page even when a full sync exceeds the former page cap", async () => {
    const pageCount = 23;
    for (let page = 1; page <= pageCount; page += 1) {
      mockApiRequest.mockResolvedValueOnce(pullPage(page, pageCount));
    }

    await expect(runSync(sandboxSession)).resolves.toMatchObject({
      pushed: 0,
      pulled: pageCount,
      conflicts: 0,
    });

    expect(mockApiRequest).toHaveBeenCalledTimes(pageCount);
    expect(mockApplyRemoteChanges).toHaveBeenCalledTimes(pageCount);
    expect(mockApiRequest).toHaveBeenLastCalledWith(
      "/sync/pull?limit=100&cursor=sandbox%3A7%3A22",
      { token: sandboxSession.token },
    );
  });

  it.each(["account", "tenant"])(
    "blocks access immediately when %s revocation is discovered in an in-flight pull",
    async (kind) => {
      const session = { ...sandboxSession, tenantId: "tenant-a" };
      const code =
        kind === "account" ? "ACCOUNT_ACCESS_CHANGED" : "TENANT_SUSPENDED";
      mockApiRequest.mockResolvedValueOnce({
        changes: [
          {
            cursor: "1",
            aggregate: kind === "account" ? "user" : "tenant_metadata",
            aggregateId:
              kind === "account" ? session.user.id : session.tenantId,
            action: "upsert",
            payload:
              kind === "account"
                ? { ...session.user, active: false }
                : { id: session.tenantId, status: "suspended" },
          },
        ],
        cursor: "1",
        hasMore: false,
      });
      await expect(runSync(session)).rejects.toMatchObject({ code });
      expect(mockNoticeAccess).toHaveBeenCalledWith(session.token, code);
      expect(mockApplyRemoteChanges).not.toHaveBeenCalled();
      expect(mockWriteSession).not.toHaveBeenCalled();
    },
  );

  it("stops safely when a paginated response does not advance its cursor", async () => {
    mockGetSyncMetadata.mockResolvedValue({ cursor: "sandbox:7:12" });
    mockApiRequest.mockResolvedValueOnce({
      changes: [],
      cursor: "sandbox:7:12",
      hasMore: true,
    });

    await expect(runSync(sandboxSession)).rejects.toMatchObject({
      code: "INVALID_SYNC_CURSOR",
    });

    expect(mockApplyRemoteChanges).not.toHaveBeenCalled();
    expect(mockSetSyncError).toHaveBeenCalledWith(
      expect.stringContaining("cursor"),
      sandboxSession,
    );
  });

  it("propagates a top-level retired-generation response", async () => {
    const retired = Object.assign(
      new Error("Generasi Mode Uji telah direset."),
      {
        status: 409,
        code: "SANDBOX_GENERATION_RETIRED",
      },
    );
    mockApiRequest.mockRejectedValueOnce(retired);

    await expect(runSync(sandboxSession)).rejects.toBe(retired);
    expect(mockSetSyncError).toHaveBeenCalledWith(
      retired.message,
      sandboxSession,
    );
  });

  it("promotes a retired-generation batch result into a recoverable API error", async () => {
    mockGetOutboxOperations
      .mockResolvedValueOnce([
        {
          operationId: "OP-1",
          aggregateId: "TRX-1",
          operation: {
            operationId: "OP-1",
            aggregate: "transaction",
            aggregateId: "TRX-1",
            action: "create",
          },
          signature: "signature",
          attempts: 0,
        },
      ])
      .mockResolvedValueOnce([]);
    mockApiRequest.mockResolvedValueOnce({
      results: [
        {
          operationId: "OP-1",
          aggregateId: "TRX-1",
          status: "rejected",
          error: {
            code: "SANDBOX_GENERATION_RETIRED",
            message: "Generasi Mode Uji telah direset.",
            requestId: "REQ-1",
          },
        },
      ],
    });

    await expect(runSync(sandboxSession)).rejects.toMatchObject({
      status: 409,
      code: "SANDBOX_GENERATION_RETIRED",
      requestId: "REQ-1",
    });

    expect(mockMarkOutboxResult).not.toHaveBeenCalled();
    expect(mockApplyRemoteChanges).not.toHaveBeenCalled();
  });

  it.each([false, true])(
    "caches the scoped tenant name and guards the durable session if context changed=%s",
    async (changed) => {
      const current = {
        ...sandboxSession,
        tenantId: "tenant-a",
        protocolVersion: 3,
        sandboxQrisPolicy: "transaction_total",
      } as Session;
      const metadata = {
        id: "tenant-a",
        name: "Nama pengelolaan baru",
        slug: "stable-slug",
        status: "active",
        revision: 2,
      };
      mockReadSession.mockResolvedValue(
        changed
          ? {
              ...current,
              token: "replacement-token",
              sessionId: "replacement-session",
            }
          : current,
      );
      mockApiRequest.mockResolvedValueOnce({
        changes: [
          {
            cursor: "9",
            aggregate: "tenant_metadata",
            aggregateId: "tenant-a",
            action: "upsert",
            payload: metadata,
            changedAt: "2026-09-12T00:00:00Z",
          },
        ],
        cursor: "9",
        hasMore: false,
      });
      await runSync(current);
      expect(mockCacheConfiguration).toHaveBeenCalledWith(
        "metadata",
        metadata,
        current,
      );
      expect(mockApplyRemoteChanges).toHaveBeenCalledWith([], "9", current);
      if (changed) expect(mockWriteSession).not.toHaveBeenCalled();
      else
        expect(mockWriteSession).toHaveBeenCalledWith(
          { ...current, tenant: metadata },
          current.sessionId,
        );
    },
  );
});
jest.mock("@/tenant/quarantine", () => ({
  blockedScopeReason: async () => null,
  quarantineScope: jest.fn(),
  SCOPE_ACCESS_CODES: new Set([
    "ACCOUNT_ACCESS_CHANGED",
    "TENANT_SUSPENDED",
    "MEMBERSHIP_INACTIVE",
    "MEMBERSHIP_REVOKED",
    "TERMINAL_REVOKED",
  ]),
}));
jest.mock("@/tenant/configuration", () => ({
  refreshTenantConfiguration: async () => undefined,
  cacheTenantConfiguration: (...args: unknown[]) =>
    mockCacheConfiguration(...args),
}));
