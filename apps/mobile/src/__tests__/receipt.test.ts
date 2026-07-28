import { encodeEscPos, formatReceipt } from "@/printer/receipt";
import type { ReceiptDocument } from "@/printer/types";

const receipt: ReceiptDocument = {
  transactionId: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
  revision: 2,
  occurredAt: "2026-07-24T03:04:05.000Z",
  cashierName: "Andi Wijaya",
  lines: [
    {
      name: "Paket Sunrise",
      unitPrice: 100_000,
      quantity: 2,
      lineTotal: 200_000,
    },
  ],
  subtotal: 200_000,
  total: 200_000,
  isCopy: false,
};

describe("thermal receipt", () => {
  it("adds the display-only transaction prefix and respects paper width", () => {
    const output = formatReceipt(receipt, 32);
    expect(output).toContain("TRX-01ARZ3NDEKTSV4RRFFQ69G5FAV");
    expect(output).toContain("TOTAL");
    expect(output).not.toContain("SALINAN");
    for (const line of output.trimEnd().split("\n")) {
      expect(line.length).toBeLessThanOrEqual(32);
    }
  });

  it("marks explicit copies and wraps output in ESC/POS init and cut bytes", () => {
    const copy = { ...receipt, isCopy: true };
    expect(formatReceipt(copy, 48)).toContain("*** SALINAN ***");
    const bytes = encodeEscPos(copy, 48);
    expect(Array.from(bytes.slice(0, 2))).toEqual([0x1b, 0x40]);
    expect(Array.from(bytes.slice(-4))).toEqual([0x1d, 0x56, 0x41, 0]);
  });
});
