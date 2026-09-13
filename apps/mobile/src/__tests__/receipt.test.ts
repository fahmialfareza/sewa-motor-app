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
  it("uses the snapshotted business identity for tenant reprints and keeps the app credit", () => {
    const snapshot = {
      businessName: "Penyewaan Merbabu",
      address: "Jl. Merbabu 12",
      phone: "08123456789",
      revision: 3,
    };
    const output = formatReceipt(
      { ...receipt, receiptIdentity: snapshot, isCopy: true },
      48,
    );
    expect(output).toContain(snapshot.businessName);
    expect(output).toContain(snapshot.address);
    expect(output).toContain(snapshot.phone);
    expect(output).toContain("Telomoyo POS");
    expect(output).toContain("SALINAN");
    expect(output).not.toContain("MODE UJI");
    const sandbox = formatReceipt(
      {
        ...receipt,
        receiptIdentity: snapshot,
        dataMode: "sandbox",
        paymentAmount: 1000,
      },
      48,
    );
    expect(sandbox).toContain(snapshot.businessName);
    expect(sandbox.match(/MODE UJI/g)).toHaveLength(1);
    expect(sandbox).toContain("Rp 1.000");
  });

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

  it("permanently marks Sandbox output compactly and preserves legacy amounts", () => {
    const sandboxReceipt = {
      ...receipt,
      dataMode: "sandbox" as const,
      paymentAmount: 1_000,
    };
    const output = formatReceipt(sandboxReceipt, 48);

    expect(output).toContain("TEST-TRX-01ARZ3NDEKTSV4RRFFQ69G5FAV");
    expect(output.match(/TEST - MODE UJI/g)).toHaveLength(1);
    expect(output.match(/BUKAN STRUK RESMI/g)).toHaveLength(1);
    expect(output).toContain("TOTAL");
    expect(output).not.toContain("TOTAL SIMULASI");
    expect(output).toContain("QRIS NYATA");
    expect(output).toContain("Rp 1.000");

    const bytes = Array.from(encodeEscPos(sandboxReceipt, 48));
    const doubleHeightCommand = [0x1d, 0x21, 0x10];
    expect(countByteSequence(bytes, doubleHeightCommand)).toBe(0);
    expect(countByteSequence(bytes, [0x1b, 0x45, 0x01])).toBe(2);
  });

  it.each([32, 48] as const)(
    "uses the full Sandbox total and fits %s columns without losing the TEST ID",
    (columns) => {
      const output = formatReceipt(
        { ...receipt, dataMode: "sandbox" },
        columns,
      );
      expect(output).toContain("TEST - MODE UJI");
      expect(output).toContain("BUKAN STRUK RESMI");
      expect(output).toContain("Rp 200.000");
      expect(output).not.toContain("QRIS NYATA");
      expect(output.replace(/\n/g, "")).toContain(
        "TEST-TRX-01ARZ3NDEKTSV4RRFFQ69G5FAV",
      );
      for (const line of output.trimEnd().split("\n")) {
        expect(line.length).toBeLessThanOrEqual(columns);
      }
    },
  );

  it("keeps the exact Production text layout unchanged", () => {
    expect(formatReceipt(receipt, 32)).toBe(
      [
        "          TELOMOYO POS",
        "--------------------------------",
        "TRX-01ARZ3NDEKTSV4RRFFQ69G5FAV",
        "Revisi 2",
        "2026-07-24T03:04:05.000Z",
        "Kasir: Putu",
        "Metode: QRIS",
        "Status: LUNAS",
        "--------------------------------",
        "Paket Sunrise",
        "2 x Rp 100.000        Rp 200.000",
        "--------------------------------",
        "Subtotal              Rp 200.000",
        "TOTAL                 Rp 200.000",
        "--------------------------------",
        "          Terima kasih",
        "",
        "",
        "",
        "",
      ].join("\n"),
    );
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
