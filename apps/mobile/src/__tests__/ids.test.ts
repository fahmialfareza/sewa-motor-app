import * as Crypto from "expo-crypto";
import { isValid } from "ulid";

import { createUlid } from "@/db/ids";

describe("mobile ULID generation", () => {
  afterEach(() => {
    jest.restoreAllMocks();
    jest.useRealTimers();
  });

  it("uses Expo Crypto instead of auto-detecting a global PRNG", () => {
    jest.useFakeTimers().setSystemTime(new Date("2026-07-28T12:00:00.000Z"));
    const getRandomBytes = jest
      .spyOn(Crypto, "getRandomBytes")
      .mockReturnValue(Uint8Array.from({ length: 16 }, (_, index) => index));

    const first = createUlid();
    const second = createUlid();

    expect(isValid(first)).toBe(true);
    expect(isValid(second)).toBe(true);
    expect(second > first).toBe(true);
    expect(getRandomBytes).toHaveBeenCalledWith(16);
  });
});
