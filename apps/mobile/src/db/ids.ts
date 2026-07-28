import * as Crypto from "expo-crypto";
import { monotonicFactory } from "ulid";

const ULID_RANDOM_LENGTH = 16;

let randomBytes: ReturnType<typeof Crypto.getRandomBytes> = new Uint8Array(0);
let randomByteIndex = 0;

function expoCryptoPrng(): number {
  if (randomByteIndex >= randomBytes.length) {
    randomBytes = Crypto.getRandomBytes(ULID_RANDOM_LENGTH);
    randomByteIndex = 0;
  }

  const randomByte = randomBytes[randomByteIndex];
  if (randomByte === undefined) {
    throw new Error("Expo Crypto tidak mengembalikan byte acak.");
  }
  randomByteIndex += 1;
  return randomByte / 256;
}

/**
 * Generates sortable ULIDs without relying on a global Web Crypto polyfill.
 * React Native randomness is supplied explicitly by Expo Crypto.
 */
export const createUlid = monotonicFactory(expoCryptoPrng);
