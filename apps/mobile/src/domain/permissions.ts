import type { Session, Transaction } from "./types";

export const CORRECTION_FORBIDDEN_MESSAGE =
  "Admin hanya dapat mengoreksi transaksi miliknya sendiri.";
export const PAYMENT_FORBIDDEN_MESSAGE =
  "Admin hanya dapat memperbarui pembayaran transaksi miliknya sendiri.";

export function canCorrectTransaction(
  session: Session | null | undefined,
  transaction: Pick<Transaction, "originActorId"> | null | undefined,
): boolean {
  if (!session || !transaction) return false;

  return (
    session.user.role === "superadmin" ||
    session.user.id === transaction.originActorId
  );
}

export const canManageTransactionPayment = canCorrectTransaction;
