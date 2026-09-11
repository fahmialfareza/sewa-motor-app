import { create } from "zustand";

import {
  INITIAL_TENANT_ID,
  type DataMode,
  type Session,
  type ContextKind,
} from "@/domain/types";

export interface ModeState {
  tenantId: string | null;
  contextKind: ContextKind;
  accessBlocked: boolean;
  dataMode: DataMode;
  dataSpaceId: string | null;
  sandboxGeneration: number | null;
  setFromSession: (session: Session | null) => void;
}

export const useModeStore = create<ModeState>((set) => ({
  tenantId: null,
  contextKind: "account",
  accessBlocked: false,
  dataMode: "production",
  dataSpaceId: null,
  sandboxGeneration: null,
  setFromSession: (session) =>
    set({
      tenantId: session
        ? (session.tenantId ??
          (!session.contextKind ? INITIAL_TENANT_ID : null))
        : null,
      contextKind: session?.contextKind ?? (session ? "tenant" : "account"),
      accessBlocked: false,
      dataMode: session?.dataMode ?? "production",
      dataSpaceId: session?.dataSpaceId ?? null,
      sandboxGeneration:
        session?.dataMode === "sandbox" ? session.sandboxGeneration : null,
    }),
}));

export function activeDataMode(): DataMode {
  return useModeStore.getState().dataMode;
}

export function activeTenantId(): string {
  const state = useModeStore.getState();
  if (state.contextKind !== "tenant" || !state.tenantId) {
    throw new Error("Pilih bisnis sebelum membuka data transaksi.");
  }
  return state.tenantId;
}

export function setModeFromSession(session: Session | null): void {
  useModeStore.getState().setFromSession(session);
}
