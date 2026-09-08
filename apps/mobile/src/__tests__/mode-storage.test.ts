import { getOrCreateDatabaseKey, readSession } from "@/security/secure-store";

const mockGetItemAsync = jest.fn();
const mockSetItemAsync = jest.fn();
const mockDeleteItemAsync = jest.fn();

jest.mock("expo-secure-store", () => ({
  WHEN_UNLOCKED_THIS_DEVICE_ONLY: "WHEN_UNLOCKED_THIS_DEVICE_ONLY",
  getItemAsync: (...args: unknown[]) => mockGetItemAsync(...args),
  setItemAsync: (...args: unknown[]) => mockSetItemAsync(...args),
  deleteItemAsync: (...args: unknown[]) => mockDeleteItemAsync(...args),
}));

jest.mock("expo-crypto", () => ({
  getRandomBytesAsync: jest.fn(),
}));

describe("mode-aware secure storage", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("upgrades a legacy session to the fixed Production data space", async () => {
    mockGetItemAsync.mockResolvedValueOnce(
      JSON.stringify({
        token: "legacy-token",
        sessionId: "LEGACY-SESSION",
        establishedAt: "2026-09-05T00:00:00.000Z",
        user: {
          id: "USER-1",
          fullName: "Putu",
          username: "putu",
          role: "admin",
          active: true,
          mustChangePassword: false,
        },
      }),
    );

    await expect(readSession()).resolves.toMatchObject({
      dataMode: "production",
      dataSpaceId: "00000000-0000-4000-8000-000000000100",
      sandboxGeneration: null,
    });
  });

  it("uses independent encryption-key entries for Production and Sandbox", async () => {
    mockGetItemAsync
      .mockResolvedValueOnce("production-key")
      .mockResolvedValueOnce("sandbox-key");

    await expect(getOrCreateDatabaseKey("production")).resolves.toBe(
      "production-key",
    );
    await expect(getOrCreateDatabaseKey("sandbox")).resolves.toBe(
      "sandbox-key",
    );

    expect(mockGetItemAsync).toHaveBeenNthCalledWith(
      1,
      "sewa-motor.database-key.v1",
    );
    expect(mockGetItemAsync).toHaveBeenNthCalledWith(
      2,
      "sewa-motor.database-key.sandbox.v1",
    );
  });
});
