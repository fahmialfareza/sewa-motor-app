import { create } from "zustand";

import type { DataMode, Session } from "@/domain/types";

export interface ModeState {
  dataMode: DataMode;
  dataSpaceId: string | null;
  sandboxGeneration: number | null;
  setFromSession: (session: Session | null) => void;
}

export const useModeStore = create<ModeState>((set) => ({
  dataMode: "production",
  dataSpaceId: null,
  sandboxGeneration: null,
  setFromSession: (session) =>
    set({
      dataMode: session?.dataMode ?? "production",
      dataSpaceId: session?.dataSpaceId ?? null,
      sandboxGeneration:
        session?.dataMode === "sandbox" ? session.sandboxGeneration : null,
    }),
}));

export function activeDataMode(): DataMode {
  return useModeStore.getState().dataMode;
}

export function setModeFromSession(session: Session | null): void {
  useModeStore.getState().setFromSession(session);
}
