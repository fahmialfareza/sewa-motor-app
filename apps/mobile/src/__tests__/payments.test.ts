import {
  isPaymentConfirmedForCurrentRevision,
  paymentMethodLabel,
  resolvePaymentAmount,
  SANDBOX_QRIS_PAYMENT_AMOUNT,
} from "@/domain/payments";

describe("payment domain", () => {
  it("only confirms payment for the exact current transaction revision", () => {
    expect(
      isPaymentConfirmedForCurrentRevision({
        revision: 3,
        paymentStatus: "success",
        paymentConfirmedRevision: 3,
      }),
    ).toBe(true);
    expect(
      isPaymentConfirmedForCurrentRevision({
        revision: 3,
        paymentStatus: "success",
        paymentConfirmedRevision: 2,
      }),
    ).toBe(false);
  });

  it("labels legacy methods without pretending they were cash or QRIS", () => {
    expect(paymentMethodLabel.legacy).toBe("Metode lama");
  });

  it("charges exactly Rp1.000 for Sandbox QRIS without changing other payments", () => {
    expect(resolvePaymentAmount("sandbox", "qris", 245_000)).toBe(
      SANDBOX_QRIS_PAYMENT_AMOUNT,
    );
    expect(resolvePaymentAmount("sandbox", "cash", 245_000)).toBe(245_000);
    expect(resolvePaymentAmount("production", "qris", 245_000)).toBe(245_000);
  });
});
