import type { PaymentMethod, PaymentStatus, Transaction } from "./types";

export const paymentMethodLabel: Record<PaymentMethod, string> = {
  cash: "Tunai",
  qris: "QRIS",
  legacy: "Metode lama",
};

export const paymentStatusLabel: Record<PaymentStatus, string> = {
  pending: "Menunggu pembayaran",
  success: "Lunas",
  failed: "Pembayaran gagal",
};

export function isPaymentConfirmedForCurrentRevision(
  transaction: Pick<
    Transaction,
    "paymentStatus" | "paymentConfirmedRevision" | "revision"
  >,
): boolean {
  return (
    transaction.paymentStatus === "success" &&
    transaction.paymentConfirmedRevision === transaction.revision
  );
}
