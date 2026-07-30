import { scanFromURLAsync, type BarcodeScanningResult } from "expo-camera";

import { validateStaticQris, type ParsedQris } from "@/domain/qris";

export type QrisImageScanner = (
  uri: string,
  barcodeTypes?: ["qr"],
) => Promise<BarcodeScanningResult[]>;

export async function readStaticQrisFromImage(
  uri: string,
  scan: QrisImageScanner = scanFromURLAsync,
): Promise<ParsedQris> {
  const normalizedUri = uri.trim();
  if (!normalizedUri) {
    throw new Error("Gambar QRIS tidak tersedia.");
  }

  let results: BarcodeScanningResult[];
  try {
    results = await scan(normalizedUri, ["qr"]);
  } catch {
    throw new Error(
      "Gambar QRIS tidak dapat dibaca. Coba gunakan gambar yang lebih jelas.",
    );
  }

  const payloads = [
    ...new Set(
      results
        .map((result) => result.data.trim())
        .filter((payload) => payload.length > 0),
    ),
  ];
  if (payloads.length === 0) {
    throw new Error(
      "Kode QR tidak ditemukan. Pastikan QRIS memenuhi sebagian besar gambar.",
    );
  }
  if (payloads.length > 1) {
    throw new Error(
      "Gambar memuat lebih dari satu kode QR. Gunakan gambar yang hanya berisi satu QRIS.",
    );
  }

  return validateStaticQris(payloads[0] ?? "");
}
