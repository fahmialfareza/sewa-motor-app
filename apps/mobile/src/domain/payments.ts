import type {
  DataMode,
  PaymentMethod,
  PaymentStatus,
  Transaction,
} from "./types";

export const SANDBOX_QRIS_PAYMENT_AMOUNT = 1_000;

export function resolvePaymentAmount(
  dataMode: DataMode,
  method: PaymentMethod,
  total: number,
): number {
  return dataMode === "sandbox" && method === "qris"
    ? SANDBOX_QRIS_PAYMENT_AMOUNT
    : total;
}

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
