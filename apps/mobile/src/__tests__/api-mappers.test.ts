import { mapApiTransaction } from "@/api/mappers";
import type { ApiTransaction } from "@/api/contracts";

const actor = {
  id: "11111111-1111-4111-8111-111111111111",
  fullName: "Andi",
  username: "andi",
  role: "admin" as const,
};

describe("API transaction mapping", () => {
  it("stores a Jakarta-offset occurrence time as canonical UTC", () => {
    const value: ApiTransaction = {
      id: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
      revision: 1,
      occurredAt: "2026-07-29T18:00:00+07:00",
      items: [],
      subtotal: 70_000,
      total: 70_000,
      paymentMethod: "qris",
      paymentStatus: "pending",
      paymentConfirmedRevision: null,
      qrisPayloadHash:
        "9185bbfe94bb008d611da515fc94c2f3ad5f0c3fbfe278d8bdb463f9ce1cf500",
      originActor: actor,
      updatedBy: actor,
      terminal: {
        id: "22222222-2222-4222-8222-222222222222",
        installationId: "33333333-3333-4333-8333-333333333333",
        name: "Kasir utama",
      },
      print: {
        state: "pending",
        attemptCount: 0,
        lastAttemptAt: null,
      },
      deletion: null,
      createdAt: "2026-07-29T11:00:00Z",
      updatedAt: "2026-07-29T11:00:00Z",
    };

    const mapped = mapApiTransaction(value);
    expect(mapped.occurredAt).toBe("2026-07-29T11:00:00.000Z");
    expect(mapped.qrisPayloadHash).toBe(value.qrisPayloadHash);
  });
});
