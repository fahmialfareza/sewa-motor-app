import { signCanonicalPayloadWithKey } from "@/security/terminal-identity";
import { canonicalize } from "@/utils/canonical-json";

function fromHex(value: string): Uint8Array {
  return Uint8Array.from(
    value.match(/.{2}/g)?.map((part) => Number.parseInt(part, 16)) ?? [],
  );
}

describe("RFC 8785 canonical signing", () => {
  it("matches the RFC 8785 reference ordering and number representation", () => {
    expect(canonicalize({ z: 0, c: 1e30, b: 4.5, a: "€" })).toBe(
      '{"a":"€","b":4.5,"c":1e+30,"z":0}',
    );
  });

  it("matches the backend mutation and Ed25519 golden vector", () => {
    const mutation = {
      operationId: "11111111-1111-4111-8111-111111111111",
      aggregate: "transaction",
      aggregateId: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
      action: "correct",
      baseRevision: 7,
      originSessionId: "22222222-2222-4222-8222-222222222222",
      originActorId: "33333333-3333-4333-8333-333333333333",
      terminalId: "44444444-4444-4444-8444-444444444444",
      occurredAt: "2026-07-24T03:04:05.000Z",
      payload: {
        reason: "Jumlah diperbaiki",
        id: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
        items: [
          {
            quantity: 2,
            packageRevision: 1,
            packageId: "00000000-0000-4000-8000-000000000001",
          },
        ],
      },
    };
    expect(canonicalize(mutation)).toBe(
      '{"action":"correct","aggregate":"transaction","aggregateId":"01ARZ3NDEKTSV4RRFFQ69G5FAV","baseRevision":7,"occurredAt":"2026-07-24T03:04:05.000Z","operationId":"11111111-1111-4111-8111-111111111111","originActorId":"33333333-3333-4333-8333-333333333333","originSessionId":"22222222-2222-4222-8222-222222222222","payload":{"id":"01ARZ3NDEKTSV4RRFFQ69G5FAV","items":[{"packageId":"00000000-0000-4000-8000-000000000001","packageRevision":1,"quantity":2}],"reason":"Jumlah diperbaiki"},"terminalId":"44444444-4444-4444-8444-444444444444"}',
    );
    expect(
      signCanonicalPayloadWithKey(
        mutation,
        fromHex(
          "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
        ),
      ),
    ).toBe(
      "g/c9NoDQdK+cE0t6NN5WKJdARDSzwxgSSNIUwD2L5EJrcJA1BR7YcI4cNwtHAkMVy6j7E8AkmvgMiN2lpjr0AA==",
    );
  });

  it("canonicalizes payment status mutations with the current revision", () => {
    const mutation = {
      operationId: "11111111-1111-4111-8111-111111111111",
      aggregate: "transaction",
      aggregateId: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
      action: "set_payment_status",
      baseRevision: 7,
      originSessionId: "22222222-2222-4222-8222-222222222222",
      originActorId: "33333333-3333-4333-8333-333333333333",
      terminalId: "44444444-4444-4444-8444-444444444444",
      occurredAt: "2026-07-24T03:04:05.000Z",
      payload: {
        id: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
        status: "success",
      },
    };

    expect(canonicalize(mutation)).toBe(
      '{"action":"set_payment_status","aggregate":"transaction","aggregateId":"01ARZ3NDEKTSV4RRFFQ69G5FAV","baseRevision":7,"occurredAt":"2026-07-24T03:04:05.000Z","operationId":"11111111-1111-4111-8111-111111111111","originActorId":"33333333-3333-4333-8333-333333333333","originSessionId":"22222222-2222-4222-8222-222222222222","payload":{"id":"01ARZ3NDEKTSV4RRFFQ69G5FAV","status":"success"},"terminalId":"44444444-4444-4444-8444-444444444444"}',
    );
  });
});
