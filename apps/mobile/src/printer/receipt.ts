import { displayTransactionId } from "@/utils/format";
import { paymentMethodLabel } from "@/domain/payments";

import type { ReceiptDocument } from "./types";

function rupiah(value: number): string {
  return `Rp ${Math.trunc(value).toLocaleString("id-ID")}`;
}

function sanitize(value: string): string {
  return value
    .normalize("NFKD")
    .replace(/[\u0300-\u036f]/g, "")
    .replace(/[^\x20-\x7E]/g, "?");
}

function center(value: string, columns: number): string {
  const clean = sanitize(value).slice(0, columns);
  const left = Math.max(0, Math.floor((columns - clean.length) / 2));
  return `${" ".repeat(left)}${clean}`;
}

function twoColumns(left: string, right: string, columns: number): string {
  const cleanLeft = sanitize(left);
  const cleanRight = sanitize(right);
  const maximumLeft = Math.max(1, columns - cleanRight.length - 1);
  const clippedLeft = cleanLeft.slice(0, maximumLeft);
  return `${clippedLeft}${" ".repeat(
    Math.max(1, columns - clippedLeft.length - cleanRight.length),
  )}${cleanRight.slice(0, columns - clippedLeft.length - 1)}`;
}

export function formatReceipt(
  document: ReceiptDocument,
  columns: 32 | 48,
): string {
  const rule = "-".repeat(columns);
  const output = [
    center("SEWA MOTOR", columns),
    center("POINT OF SALE", columns),
    document.isCopy ? center("*** SALINAN ***", columns) : "",
    rule,
    displayTransactionId(document.transactionId),
    `Revisi ${document.revision}`,
    new Date(document.occurredAt).toISOString(),
    `Kasir: ${sanitize(document.cashierName)}`,
    `Metode: ${paymentMethodLabel[document.paymentMethod].toUpperCase()}`,
    "Status: LUNAS",
    rule,
  ].filter(Boolean);

  for (const line of document.lines) {
    output.push(sanitize(line.name).slice(0, columns));
    output.push(
      twoColumns(
        `${line.quantity} x ${rupiah(line.unitPrice)}`,
        rupiah(line.lineTotal),
        columns,
      ),
    );
  }

  output.push(
    rule,
    twoColumns("Subtotal", rupiah(document.subtotal), columns),
    twoColumns("TOTAL", rupiah(document.total), columns),
    rule,
    center("Terima kasih", columns),
    "",
    "",
    "",
  );
  return `${output.join("\n")}\n`;
}

export function encodeEscPos(
  document: ReceiptDocument,
  columns: 32 | 48,
): Uint8Array {
  const text = formatReceipt(document, columns);
  const textBytes = Uint8Array.from(
    Array.from(text, (character) => {
      const code = character.charCodeAt(0);
      return code >= 32 || code === 10 ? Math.min(code, 126) : 63;
    }),
  );
  const initialize = Uint8Array.from([0x1b, 0x40]);
  const cut = Uint8Array.from([0x1d, 0x56, 0x41, 0x00]);
  const result = new Uint8Array(
    initialize.length + textBytes.length + cut.length,
  );
  result.set(initialize, 0);
  result.set(textBytes, initialize.length);
  result.set(cut, initialize.length + textBytes.length);
  return result;
}
