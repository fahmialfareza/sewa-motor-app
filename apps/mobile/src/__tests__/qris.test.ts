import * as Crypto from "expo-crypto";

import {
  calculateQrisCrc,
  createDynamicQris,
  fingerprintStaticQris,
  normalizeQrisPayloadHash,
  parseQris,
  validateQrisPayloadBinding,
  validateStaticQris,
} from "@/domain/qris";

jest.mock("expo-crypto", () => ({
  CryptoDigestAlgorithm: { SHA256: "SHA-256" },
  CryptoEncoding: { HEX: "hex" },
  digestStringAsync: jest.fn(() =>
    Promise.resolve(
      "9185BBFE94BB008D611DA515FC94C2F3AD5F0C3FBFE278D8BDB463F9CE1CF500",
    ),
  ),
}));

const STATIC_QRIS =
  "00020101021126320014ID.CO.TEST.WWW011012345678905204729953033605802ID5910SEWA MOTOR6008DENPASAR6304AA64";
const DYNAMIC_QRIS_70K =
  "00020101021226320014ID.CO.TEST.WWW011012345678905204729953033605405700005802ID5910SEWA MOTOR6008DENPASAR63048886";

function withCrc(payloadWithoutCrc: string): string {
  const input = `${payloadWithoutCrc}6304`;
  return `${input}${calculateQrisCrc(input)}`;
}

describe("QRIS conversion", () => {
  it("parses and validates a static merchant payload", () => {
    expect(parseQris(`\n${STATIC_QRIS}\n`)).toEqual({
      payload: STATIC_QRIS,
      pointOfInitiation: "static",
      currency: "360",
      countryCode: "ID",
      merchantName: "SEWA MOTOR",
      merchantCity: "DENPASAR",
      amount: null,
    });
  });

  it("creates the expected amount-specific dynamic payload and CRC", () => {
    expect(createDynamicQris(STATIC_QRIS, 70_000)).toEqual({
      payload: DYNAMIC_QRIS_70K,
      pointOfInitiation: "dynamic",
      currency: "360",
      countryCode: "ID",
      merchantName: "SEWA MOTOR",
      merchantCity: "DENPASAR",
      amount: "70000",
    });
  });

  it("fingerprints the normalized static payload as lowercase SHA-256 hex", async () => {
    await expect(fingerprintStaticQris(`\n${STATIC_QRIS}\n`)).resolves.toBe(
      "9185bbfe94bb008d611da515fc94c2f3ad5f0c3fbfe278d8bdb463f9ce1cf500",
    );
    expect(Crypto.digestStringAsync).toHaveBeenCalledWith(
      Crypto.CryptoDigestAlgorithm.SHA256,
      STATIC_QRIS,
      { encoding: Crypto.CryptoEncoding.HEX },
    );
  });

  it("requires QRIS bindings and rejects them for cash", () => {
    const uppercaseHash =
      "9185BBFE94BB008D611DA515FC94C2F3AD5F0C3FBFE278D8BDB463F9CE1CF500";
    const lowercaseHash = uppercaseHash.toLowerCase();
    expect(normalizeQrisPayloadHash(uppercaseHash)).toBeNull();
    expect(normalizeQrisPayloadHash(lowercaseHash)).toBe(lowercaseHash);
    expect(() => validateQrisPayloadBinding("qris", uppercaseHash)).toThrow(
      "wajib tersedia",
    );
    expect(() => validateQrisPayloadBinding("qris", null)).toThrow(
      "wajib tersedia",
    );
    expect(() => validateQrisPayloadBinding("cash", uppercaseHash)).toThrow(
      "hanya boleh disimpan",
    );
  });

  it("rejects transaction-specific amount and fee tags in static config", () => {
    const staticWithTransactionFields = withCrc(
      "00020101021126320014ID.CO.TEST.WWW0110123456789052047299530336054051000055020256035005802ID5910SEWA MOTOR6008DENPASAR",
    );
    expect(() => validateStaticQris(staticWithTransactionFields)).toThrow(
      "tidak boleh memuat tag transaksi 54",
    );
  });

  it.each([
    ["payload kosong", "", "tidak boleh kosong"],
    ["checksum salah", `${STATIC_QRIS.slice(0, -1)}0`, "Checksum CRC"],
    [
      "payload dinamis sebagai konfigurasi",
      DYNAMIC_QRIS_70K,
      "harus menggunakan QRIS MPM statis",
    ],
    [
      "mata uang bukan IDR",
      withCrc(
        "00020101021126320014ID.CO.TEST.WWW011012345678905204729953038405802ID5910SEWA MOTOR6008DENPASAR",
      ),
      "mata uang IDR",
    ],
  ])("rejects %s", (_, payload, expectedMessage) => {
    const action = () =>
      expectedMessage.includes("MPM")
        ? createDynamicQris(payload, 10_000)
        : parseQris(payload);
    expect(action).toThrow(expectedMessage);
  });

  it.each([0, -1, 1.5, 10_000_001, Number.NaN, Number.POSITIVE_INFINITY])(
    "rejects invalid amount %s",
    (amount) => {
      expect(() => createDynamicQris(STATIC_QRIS, amount)).toThrow(
        "antara Rp1 dan Rp10.000.000",
      );
    },
  );

  it("rejects truncated TLV instead of partially parsing it", () => {
    expect(() => parseQris(STATIC_QRIS.slice(0, -5))).toThrow(
      /terpotong|tidak ditemukan/,
    );
  });

  it("uses the CRC-16/CCITT-FALSE standard check value", () => {
    expect(calculateQrisCrc("123456789")).toBe("29B1");
  });

  it("rejects duplicate root tags and invalid ordering", () => {
    expect(() =>
      parseQris(
        withCrc(
          "00020101021101021126320014ID.CO.TEST.WWW011012345678905204729953033605802ID5910SEWA MOTOR6008DENPASAR",
        ),
      ),
    ).toThrow("tidak boleh duplikat");
    expect(() =>
      parseQris(
        withCrc(
          "00020101021126320014ID.CO.TEST.WWW011012345678905303360520472995802ID5910SEWA MOTOR6008DENPASAR",
        ),
      ),
    ).toThrow("Urutan tag");
  });
});
