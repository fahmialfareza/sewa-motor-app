import type { Session } from "@/domain/types";

import { useModeStore } from "./mode-store";

export const MODE_TRANSITION_BUSY_MESSAGE =
  "Pergantian mode sedang berlangsung. Tunggu hingga selesai sebelum mengubah data.";

export const STALE_DATA_SPACE_MESSAGE =
  "Layar ini berasal dari ruang data yang sudah tidak aktif. Muat ulang sebelum mengubah data.";

export interface ModeTransitionLease {
  readonly id: symbol;
  release: () => void;
}

let activeTransition: ModeTransitionLease | null = null;
let activeLocalMutations = 0;
let resolveDrained: (() => void) | null = null;
const transitionReleaseWaiters = new Set<() => void>();
const localLeases = new WeakSet<() => void>();

export function isLocalMutationLeaseActive(lease: () => void): boolean {
  return localLeases.has(lease);
}

/**
 * Reserves admission for one user-originated local write. The returned lease
 * must be released after the whole mutation (including signing) completes.
 */
export function beginLocalMutation(
  session?: Pick<Session, "dataMode" | "dataSpaceId" | "sandboxGeneration"> &
    Partial<Pick<Session, "tenantId">>,
): () => void {
  if (activeTransition) {
    throw new Error(MODE_TRANSITION_BUSY_MESSAGE);
  }

  if (session) {
    const active = useModeStore.getState();
    if (
      active.accessBlocked ||
      active.contextKind !== "tenant" ||
      (session.tenantId != null && active.tenantId !== session.tenantId) ||
      active.dataMode !== session.dataMode ||
      active.dataSpaceId !== session.dataSpaceId ||
      active.sandboxGeneration !== session.sandboxGeneration
    ) {
      throw new Error(STALE_DATA_SPACE_MESSAGE);
    }
  }
  if (useModeStore.getState().accessBlocked)
    throw new Error(
      "Akses bisnis dihentikan. Data belum tersinkron telah diamankan.",
    );
  return reserveLocalLease();
}

function reserveLocalLease(): () => void {
  activeLocalMutations += 1;
  let released = false;
  const release = () => {
    if (released) return;
    released = true;
    localLeases.delete(release);
    activeLocalMutations -= 1;
    if (activeTransition && activeLocalMutations === 0) {
      const resolve = resolveDrained;
      resolveDrained = null;
      resolve?.();
    }
  };
  localLeases.add(release);
  return release;
}

/**
 * Closes mutation admission synchronously, then waits for leases that were
 * already admitted. Callers may safely drain the outbox after this resolves.
 */
export async function beginModeTransition(): Promise<ModeTransitionLease> {
  if (activeTransition) {
    throw new Error(MODE_TRANSITION_BUSY_MESSAGE);
  }

  let released = false;
  const lease: ModeTransitionLease = {
    id: Symbol("mode-transition"),
    release: () => {
      if (released) return;
      released = true;
      if (activeTransition === lease) {
        activeTransition = null;
        const waiters = [...transitionReleaseWaiters];
        transitionReleaseWaiters.clear();
        for (const resolve of waiters) resolve();
      }
    },
  };
  activeTransition = lease;

  if (activeLocalMutations > 0) {
    await new Promise<void>((resolve) => {
      resolveDrained = resolve;
    });
  }

  return lease;
}

/**
 * Reserves access to mode-scoped local state after any active transition.
 * This is used during authentication hydration so a background recovery
 * cannot close or replace SQLite while startup is reading it.
 */
export async function beginModeSafeLocalAccess(): Promise<() => void> {
  while (activeTransition) {
    await new Promise<void>((resolve) => {
      transitionReleaseWaiters.add(resolve);
    });
  }
  return reserveLocalLease();
}

export function isModeTransitionActive(): boolean {
  return activeTransition !== null;
}

export function isModeTransitionLeaseActive(
  lease: ModeTransitionLease,
): boolean {
  return activeTransition === lease;
}

export function resetMutationBarrierForTests(): void {
  const resolvePendingDrain = resolveDrained;
  activeTransition = null;
  activeLocalMutations = 0;
  resolveDrained = null;
  resolvePendingDrain?.();
  const waiters = [...transitionReleaseWaiters];
  transitionReleaseWaiters.clear();
  for (const resolve of waiters) resolve();
}
