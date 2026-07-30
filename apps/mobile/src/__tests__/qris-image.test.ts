import type { BarcodeScanningResult } from "expo-camera";

import {
  readStaticQrisFromImage,
  type QrisImageScanner,
} from "@/domain/qris-image";

jest.mock("expo-camera", () => ({
  scanFromURLAsync: jest.fn(),
}));

const STATIC_QRIS =
  "00020101021126320014ID.CO.TEST.WWW011012345678905204729953033605802ID5910SEWA MOTOR6008DENPASAR6304AA64";
const DYNAMIC_QRIS =
  "00020101021226320014ID.CO.TEST.WWW011012345678905204729953033605405700005802ID5910SEWA MOTOR6008DENPASAR63048886";

function barcode(data: string): BarcodeScanningResult {
  return { data } as BarcodeScanningResult;
}

function scanner(
  result: BarcodeScanningResult[] | Error,
): jest.MockedFunction<QrisImageScanner> {
  return jest.fn<ReturnType<QrisImageScanner>, Parameters<QrisImageScanner>>(
    async () => {
      if (result instanceof Error) throw result;
      return result;
    },
  );
}

describe("QRIS image reader", () => {
  it("reads and validates one static QRIS from a local image", async () => {
    const scan = scanner([barcode(STATIC_QRIS)]);

    await expect(
      readStaticQrisFromImage(" file:///qris.jpg ", scan),
    ).resolves.toMatchObject({
      payload: STATIC_QRIS,
      pointOfInitiation: "static",
      merchantName: "SEWA MOTOR",
      merchantCity: "DENPASAR",
    });
    expect(scan).toHaveBeenCalledWith("file:///qris.jpg", ["qr"]);
  });

  it("deduplicates repeated detections of the same QR", async () => {
    const scan = scanner([barcode(STATIC_QRIS), barcode(` ${STATIC_QRIS} `)]);

    await expect(
      readStaticQrisFromImage("file:///qris.jpg", scan),
    ).resolves.toMatchObject({
      payload: STATIC_QRIS,
    });
  });

  it("rejects images without a QR code", async () => {
    await expect(
      readStaticQrisFromImage("file:///empty.jpg", scanner([])),
    ).rejects.toThrow("Kode QR tidak ditemukan");
  });

  it("rejects an ambiguous image with multiple distinct QR codes", async () => {
    await expect(
      readStaticQrisFromImage(
        "file:///multiple.jpg",
        scanner([barcode(STATIC_QRIS), barcode("https://example.com")]),
      ),
    ).rejects.toThrow("lebih dari satu kode QR");
  });

  it("rejects a dynamic payload as merchant configuration", async () => {
    await expect(
      readStaticQrisFromImage(
        "file:///dynamic.jpg",
        scanner([barcode(DYNAMIC_QRIS)]),
      ),
    ).rejects.toThrow("harus menggunakan QRIS MPM statis");
  });

  it("turns native decoding failures into a useful retry message", async () => {
    await expect(
      readStaticQrisFromImage(
        "file:///broken.jpg",
        scanner(new Error("native decode failed")),
      ),
    ).rejects.toThrow("gambar yang lebih jelas");
  });
});
