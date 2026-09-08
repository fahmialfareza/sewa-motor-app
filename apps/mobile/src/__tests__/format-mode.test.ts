import { compactTransactionId, displayTransactionId } from "@/utils/format";

describe("mode-aware transaction IDs", () => {
  it("uses TEST-TRX in Sandbox without duplicating an existing prefix", () => {
    expect(displayTransactionId("01ARZ3", "sandbox")).toBe("TEST-TRX-01ARZ3");
    expect(displayTransactionId("TRX-01ARZ3", "sandbox")).toBe(
      "TEST-TRX-01ARZ3",
    );
    expect(displayTransactionId("TEST-TRX-01ARZ3", "sandbox")).toBe(
      "TEST-TRX-01ARZ3",
    );
  });

  it("keeps compact Sandbox IDs visibly marked as TEST", () => {
    expect(
      compactTransactionId("01ARZ3NDEKTSV4RRFFQ69G5FAV", "sandbox"),
    ).toMatch(/^TEST-TRX-/);
  });
});
