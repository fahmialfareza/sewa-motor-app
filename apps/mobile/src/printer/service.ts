import { readPrinterConfig, type PrinterConfig } from "@/security/secure-store";

import { BluetoothEscPosPrinter, IntegratedVendorPrinter } from "./adapters";
import { SimulatorPrinter } from "./simulator";
import type { ReceiptPrinter } from "./types";

export function createPrinter(config: PrinterConfig): ReceiptPrinter {
  if (config.adapter === "bluetooth") {
    return new BluetoothEscPosPrinter(config.paperColumns);
  }
  if (config.adapter === "integrated") {
    return new IntegratedVendorPrinter(config.paperColumns);
  }
  return new SimulatorPrinter("success", config.paperColumns);
}

export async function getConfiguredPrinter(): Promise<{
  printer: ReceiptPrinter;
  config: PrinterConfig;
}> {
  const config = await readPrinterConfig();
  return { config, printer: createPrinter(config) };
}
