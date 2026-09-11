import type { Session } from "@/domain/types";
import { setModeFromSession, useModeStore } from "@/mode/mode-store";
import {
  beginLocalMutation,
  beginModeSafeLocalAccess,
  beginModeTransition,
  isModeTransitionActive,
  isLocalMutationLeaseActive,
  MODE_TRANSITION_BUSY_MESSAGE,
  resetMutationBarrierForTests,
  STALE_DATA_SPACE_MESSAGE,
} from "@/mode/mutation-barrier";

const productionSession: Session = {
  token: "production-token",
  sessionId: "PRODUCTION-SESSION",
  establishedAt: "2026-09-08T00:00:00.000Z",
  dataMode: "production",
  dataSpaceId: "00000000-0000-4000-8000-000000000100",
  sandboxGeneration: null,
  contextKind: "tenant",
  tenantId: "00000000-0000-4000-8000-000000000200",
  user: {
    id: "USER-1",
    fullName: "Putu",
    username: "putu",
    role: "admin",
    active: true,
    mustChangePassword: false,
  },
};

describe("mode-transition local mutation barrier", () => {
  beforeEach(() => {
    resetMutationBarrierForTests();
    setModeFromSession(productionSession);
  });

  afterEach(() => {
    resetMutationBarrierForTests();
    setModeFromSession(null);
  });

  it("closes admission before waiting for an already-active mutation", async () => {
    const releaseMutation = beginLocalMutation(productionSession);
    let transitionAcquired = false;
    const transition = beginModeTransition().then((lease) => {
      transitionAcquired = true;
      return lease;
    });

    expect(isModeTransitionActive()).toBe(true);
    expect(transitionAcquired).toBe(false);
    expect(() => beginLocalMutation(productionSession)).toThrow(
      MODE_TRANSITION_BUSY_MESSAGE,
    );

    releaseMutation();
    const transitionLease = await transition;
    expect(transitionAcquired).toBe(true);

    transitionLease.release();
    expect(isModeTransitionActive()).toBe(false);
    expect(() => beginLocalMutation(productionSession)).not.toThrow();
  });

  it("rejects a delayed mutation carrying credentials for an old data space", () => {
    setModeFromSession({
      ...productionSession,
      token: "sandbox-token",
      sessionId: "SANDBOX-SESSION",
      dataMode: "sandbox",
      dataSpaceId: "00000000-0000-4000-8000-000000000207",
      sandboxGeneration: 7,
    });

    expect(() => beginLocalMutation(productionSession)).toThrow(
      STALE_DATA_SPACE_MESSAGE,
    );
  });

  it("delays startup local access until a transition releases its lease", async () => {
    const transitionLease = await beginModeTransition();
    let accessAcquired = false;
    const pendingAccess = beginModeSafeLocalAccess().then((release) => {
      accessAcquired = true;
      return release;
    });

    await Promise.resolve();
    expect(accessAcquired).toBe(false);

    transitionLease.release();
    const releaseAccess = await pendingAccess;
    expect(accessAcquired).toBe(true);

    const nextTransition = beginModeTransition();
    await Promise.resolve();
    expect(isModeTransitionActive()).toBe(true);
    releaseAccess();
    (await nextTransition).release();
  });

  it("rejects a stale tenant even if a malformed callback carries the current space ID", () => {
    const secondTenantSession = {
      ...productionSession,
      tenantId: "00000000-0000-4000-8000-000000000201",
    };
    setModeFromSession(secondTenantSession);
    expect(() => beginLocalMutation(productionSession)).toThrow(
      STALE_DATA_SPACE_MESSAGE,
    );
    const release = beginLocalMutation(secondTenantSession);
    release();
  });

  it.each(["account", "platform"] as const)(
    "does not admit business mutations in %s context",
    (contextKind) => {
      setModeFromSession({
        ...productionSession,
        contextKind,
        tenantId: null,
        dataSpaceId: null,
      });
      expect(() => beginLocalMutation(productionSession)).toThrow(
        STALE_DATA_SPACE_MESSAGE,
      );
    },
  );

  it("blocks new work in a quarantined scope", () => {
    useModeStore.setState({ accessBlocked: true });
    expect(() => beginLocalMutation(productionSession)).toThrow(
      STALE_DATA_SPACE_MESSAGE,
    );
    expect(() => beginLocalMutation()).toThrow(
      "Data belum tersinkron telah diamankan",
    );
  });

  it("holds a tenant switch until the entire physical-print lease and other writes finish", async () => {
    // The printer owns the same lease from document freezing through hardware
    // completion and recording the print attempt, not just while SQLite writes.
    const releasePrint = beginLocalMutation(productionSession);
    const releaseWrite = beginLocalMutation(productionSession);
    let switched = false;
    const switchLease = beginModeTransition().then((lease) => {
      switched = true;
      return lease;
    });

    expect(isLocalMutationLeaseActive(releasePrint)).toBe(true);
    expect(() => beginLocalMutation(productionSession)).toThrow(
      MODE_TRANSITION_BUSY_MESSAGE,
    );
    releaseWrite();
    releaseWrite(); // Idempotent cleanup must not consume the printer's lease.
    await Promise.resolve();
    expect(switched).toBe(false);
    expect(isLocalMutationLeaseActive(releasePrint)).toBe(true);

    releasePrint();
    const transition = await switchLease;
    expect(switched).toBe(true);
    expect(isLocalMutationLeaseActive(releasePrint)).toBe(false);
    transition.release();
  });
});
