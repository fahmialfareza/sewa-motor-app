import { encodeEscPos, formatReceipt } from "@/printer/receipt";
import type { ReceiptDocument } from "@/printer/types";

const receipt: ReceiptDocument = {
  transactionId: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
  revision: 2,
  occurredAt: "2026-07-24T03:04:05.000Z",
  cashierName: "Putu",
  paymentMethod: "qris",
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
  paymentAmount: 200_000,
  dataMode: "production",
  isCopy: false,
};

describe("thermal receipt", () => {
  it("adds the display-only transaction prefix and respects paper width", () => {
    const output = formatReceipt(receipt, 32);
    expect(output).toContain("TRX-01ARZ3NDEKTSV4RRFFQ69G5FAV");
    expect(output).toContain("TOTAL");
    expect(output).toContain("Metode: QRIS");
    expect(output).toContain("Status: LUNAS");
    expect(output).toContain("TELOMOYO POS");
    expect(output).not.toContain("SEWA MOTOR POS");
    expect(output).not.toContain("SEWA MOTOR\n");
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

  it("permanently marks Sandbox output and separates simulated and real amounts", () => {
    const sandboxReceipt = {
      ...receipt,
      dataMode: "sandbox" as const,
      paymentAmount: 1_000,
    };
    const output = formatReceipt(sandboxReceipt, 48);

    expect(output).toContain("TEST-TRX-01ARZ3NDEKTSV4RRFFQ69G5FAV");
    expect(output.match(/MODE UJI/g)).toHaveLength(2);
    expect(output.match(/BUKAN STRUK RESMI/g)).toHaveLength(2);
    expect(output).toContain("TOTAL SIMULASI");
    expect(output).toContain("QRIS NYATA");
    expect(output).toContain("Rp 1.000");

    const bytes = Array.from(encodeEscPos(sandboxReceipt, 48));
    const doubleHeightCommand = [0x1d, 0x21, 0x10];
    expect(countByteSequence(bytes, doubleHeightCommand)).toBe(4);
  });
});

function countByteSequence(bytes: number[], sequence: number[]): number {
  let count = 0;
  for (let index = 0; index <= bytes.length - sequence.length; index += 1) {
    if (sequence.every((value, offset) => bytes[index + offset] === value)) {
      count += 1;
    }
  }
  return count;
}
