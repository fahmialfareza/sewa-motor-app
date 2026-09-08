import type { Session } from "@/domain/types";
import { setModeFromSession } from "@/mode/mode-store";
import {
  beginLocalMutation,
  beginModeSafeLocalAccess,
  beginModeTransition,
  isModeTransitionActive,
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
});
