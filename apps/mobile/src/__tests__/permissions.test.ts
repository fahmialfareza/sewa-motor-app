import {
  canCorrectTransaction,
  CORRECTION_FORBIDDEN_MESSAGE,
} from "@/domain/permissions";
import type { Session, Transaction } from "@/domain/types";

function session(id: string, role: "admin" | "superadmin"): Session {
  return {
    token: "token",
    sessionId: "session",
    establishedAt: "2026-07-28T00:00:00.000Z",
    user: {
      id,
      fullName: "User",
      username: id,
      role,
      active: true,
      mustChangePassword: false,
    },
  };
}

const ownedTransaction = {
  originActorId: "owner",
} as Pick<Transaction, "originActorId">;

describe("transaction correction permissions", () => {
  it("allows an admin to correct their own transaction", () => {
    expect(
      canCorrectTransaction(session("owner", "admin"), ownedTransaction),
    ).toBe(true);
  });

  it("rejects an admin correcting another user's transaction", () => {
    expect(
      canCorrectTransaction(session("other", "admin"), ownedTransaction),
    ).toBe(false);
    expect(CORRECTION_FORBIDDEN_MESSAGE).toContain("miliknya sendiri");
  });

  it("allows a superadmin to correct any transaction", () => {
    expect(
      canCorrectTransaction(session("other", "superadmin"), ownedTransaction),
    ).toBe(true);
  });

  it("rejects correction without an active session or transaction", () => {
    expect(canCorrectTransaction(null, ownedTransaction)).toBe(false);
    expect(canCorrectTransaction(session("owner", "admin"), null)).toBe(false);
  });
});
