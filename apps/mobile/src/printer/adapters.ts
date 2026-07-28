import { bytesToBase64 } from "@/security/secure-store";

import { encodeEscPos } from "./receipt";
import { SewaPrinterNative } from "./native";
import type {
  PrinterDevice,
  PrinterResult,
  PrinterStatus,
  ReceiptDocument,
  ReceiptPrinter,
} from "./types";

export class BluetoothEscPosPrinter implements ReceiptPrinter {
  readonly kind = "bluetooth" as const;

  constructor(private readonly columns: 32 | 48) {}

  async discover(): Promise<PrinterDevice[]> {
    const devices = await SewaPrinterNative.discoverBluetooth();
    return devices.map((device: { id: string; name: string }) => ({
      ...device,
      kind: this.kind,
    }));
  }

  async connect(deviceId?: string): Promise<void> {
    if (!deviceId) throw new Error("Pilih printer Bluetooth.");
    await SewaPrinterNative.connectBluetooth(deviceId);
  }

  status(): Promise<PrinterStatus> {
    return SewaPrinterNative.getBluetoothStatus();
  }

  async print(document: ReceiptDocument): Promise<PrinterResult> {
    const status = await this.status();
    if (!status.ready) return { status: "failed", message: status.message };
    const bytes = encodeEscPos(document, this.columns);
    try {
      const written = await SewaPrinterNative.writeBluetooth(
        bytesToBase64(bytes),
      );
      return written === bytes.length
        ? { status: "success" }
        : {
            status: "unknown",
            message:
              "Jumlah byte yang diterima printer tidak dapat dipastikan.",
          };
    } catch (error) {
      return {
        status: "unknown",
        message:
          error instanceof Error
            ? error.message
            : "Koneksi terputus saat mencetak.",
      };
    }
  }

  async disconnect(): Promise<void> {
    await SewaPrinterNative.disconnectBluetooth();
  }
}

export class IntegratedVendorPrinter implements ReceiptPrinter {
  readonly kind = "integrated" as const;

  constructor(private readonly columns: 32 | 48) {}

  async discover(): Promise<PrinterDevice[]> {
    const status = await this.status();
    return status.ready
      ? [{ id: "integrated", name: "Printer MPOS", kind: this.kind }]
      : [];
  }

  async connect(): Promise<void> {
    const status = await this.status();
    if (!status.ready) throw new Error(status.message);
  }

  status(): Promise<PrinterStatus> {
    return SewaPrinterNative.getIntegratedPrinterStatus();
  }

  async print(document: ReceiptDocument): Promise<PrinterResult> {
    try {
      await SewaPrinterNative.printIntegrated(
        bytesToBase64(encodeEscPos(document, this.columns)),
      );
      return { status: "success" };
    } catch (error) {
      return {
        status: "failed",
        message:
          error instanceof Error
            ? error.message
            : "Printer terintegrasi gagal mencetak.",
      };
    }
  }

  async disconnect(): Promise<void> {}
}
