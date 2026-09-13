import { SimulatorPrinter } from "@/printer/simulator";
import { formatReceipt } from "@/printer/receipt";
import type { ReceiptDocument } from "@/printer/types";

const document: ReceiptDocument = {
  transactionId: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
  revision: 1,
  occurredAt: "2026-07-24T03:04:05.000Z",
  cashierName: "Andi",
  paymentMethod: "cash",
  lines: [
    {
      name: "Paket Standar",
      unitPrice: 70_000,
      quantity: 1,
      lineTotal: 70_000,
    },
  ],
  subtotal: 70_000,
  total: 70_000,
  paymentAmount: 70_000,
  dataMode: "production",
  isCopy: false,
};

describe("printer simulator", () => {
  it.each([32, 48] as const)(
    "simulates the exact full-value Sandbox preview at %s columns",
    async (columns) => {
      const sandbox = {
        ...document,
        dataMode: "sandbox" as const,
        paymentMethod: "qris" as const,
      };
      const printer = new SimulatorPrinter("success", columns);
      await printer.connect();
      await expect(printer.print(sandbox)).resolves.toEqual({
        status: "success",
      });
      expect(printer.lastOutput).toBe(formatReceipt(sandbox, columns));
      expect(printer.lastOutput).toContain("TEST - MODE UJI");
      expect(printer.lastOutput).toContain("BUKAN STRUK RESMI");
      expect(printer.lastOutput).toContain("Rp 70.000");
      await printer.disconnect();
    },
  );
  it("requires a connection before printing", async () => {
    const printer = new SimulatorPrinter();
    await expect(printer.print(document)).resolves.toEqual({
      status: "failed",
      message: "Simulator belum tersambung.",
    });
  });

  it.each(["success", "failed", "unknown"] as const)(
    "returns a deterministic %s outcome",
    async (outcome) => {
      const printer = new SimulatorPrinter(outcome);
      await printer.connect();
      const result = await printer.print(document);
      expect(result.status).toBe(outcome);
      expect(printer.lastOutput).toContain("TRX-01ARZ3NDEKTSV4RRFFQ69G5FAV");
      await printer.disconnect();
      await expect(printer.status()).resolves.toMatchObject({
        connected: false,
        ready: false,
      });
    },
  );
});
