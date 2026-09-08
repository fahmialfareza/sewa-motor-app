import type { Session } from "@/domain/types";

import type { SyncSummary } from "./engine";

interface SyncStateHandoff {
  reset: (session: Session | null) => void;
  hydrate: (
    session: Session | null,
    summary?: SyncSummary | null,
  ) => Promise<void>;
}

let activeHandoff: SyncStateHandoff | null = null;

const UNAVAILABLE_MESSAGE =
  "Status sinkronisasi untuk ruang data baru belum dapat disiapkan.";

export function registerSyncStateHandoff(handoff: SyncStateHandoff): void {
  activeHandoff = handoff;
}

export function resetSyncStateForSession(session: Session | null): void {
  if (!activeHandoff) throw new Error(UNAVAILABLE_MESSAGE);
  activeHandoff.reset(session);
}

export function hydrateSyncStateForSession(
  session: Session | null,
  summary: SyncSummary | null = null,
): Promise<void> {
  if (!activeHandoff) return Promise.reject(new Error(UNAVAILABLE_MESSAGE));
  return activeHandoff.hydrate(session, summary);
}
