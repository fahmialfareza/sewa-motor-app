import {
  isPaymentConfirmedForCurrentRevision,
  paymentMethodLabel,
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
});
