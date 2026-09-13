import { clearSession, writeSession } from "@/security/secure-store";
import type { Session } from "@/domain/types";

const mockStorage = new Map<string, string>();
const mockWrite = jest.fn(async (key: string, value: string) => {
  mockStorage.set(key, value);
});
const mockDelete = jest.fn(async (key: string) => {
  mockStorage.delete(key);
});
jest.mock("expo-secure-store", () => ({
  WHEN_UNLOCKED_THIS_DEVICE_ONLY: "device-only",
  getItemAsync: async (key: string) => mockStorage.get(key) ?? null,
  setItemAsync: (...args: [string, string]) => mockWrite(...args),
  deleteItemAsync: (key: string) => mockDelete(key),
}));

const original = {
  sessionId: "old-session",
  token: "old-token",
  sandboxQrisPolicy: "fixed_1000",
} as Session;
const replacement = {
  sessionId: "new-session",
  token: "new-token",
  sandboxQrisPolicy: "transaction_total",
} as Session;
const sessionKey = "sewa-motor.session.v1";

describe("serialized session persistence", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockStorage.clear();
  });

  it("does not delete a new account when delayed revocation clears the old token", async () => {
    await writeSession(original);
    await Promise.all([
      writeSession(replacement),
      clearSession(original.token),
    ]);
    expect(JSON.parse(mockStorage.get(sessionKey)!)).toEqual(replacement);
    expect(mockDelete).not.toHaveBeenCalled();
  });

  it("does not restore an old policy or context from a background profile refresh", async () => {
    await writeSession(original);
    await Promise.all([
      writeSession(replacement),
      writeSession(original, original.sessionId),
    ]);
    expect(JSON.parse(mockStorage.get(sessionKey)!)).toEqual(replacement);
  });

  it("still clears the exact revoked token and permits subsequent writes", async () => {
    await writeSession(original);
    await clearSession(original.token);
    expect(mockStorage.has(sessionKey)).toBe(false);
    mockWrite.mockRejectedValueOnce(
      new Error("storage temporarily unavailable"),
    );
    await expect(writeSession(original)).rejects.toThrow(
      "storage temporarily unavailable",
    );
    await writeSession(replacement);
    expect(JSON.parse(mockStorage.get(sessionKey)!)).toEqual(replacement);
  });
});
