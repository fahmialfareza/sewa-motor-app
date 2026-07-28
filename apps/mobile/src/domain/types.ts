export type Role = "admin" | "superadmin";
export type SyncState = "pending" | "synced" | "conflict" | "error";
export type PrintState =
  | "pending"
  | "success"
  | "failed"
  | "unknown"
  | "needs-reprint";

export interface UserSummary {
  id: string;
  fullName: string;
  username: string;
  role: Role;
  active: boolean;
  mustChangePassword: boolean;
}

export interface Session {
  token: string;
  sessionId: string;
  user: UserSummary;
  establishedAt: string;
}

export interface RentalPackage {
  id: string;
  revision: number;
  name: string;
  description: string;
  unitPrice: number;
  accent: "standard" | "sunrise" | "primary";
  active: boolean;
  deletedAt: string | null;
}

export interface TransactionItem {
  id: string;
  packageId: string;
  packageRevision: number;
  name: string;
  description: string;
  accent: RentalPackage["accent"];
  unitPrice: number;
  quantity: number;
  lineTotal: number;
}

export interface Transaction {
  id: string;
  revision: number;
  occurredAt: string;
  subtotal: number;
  total: number;
  originActorId: string;
  originActorName: string;
  updatedActorName: string;
  terminalId: string;
  syncState: SyncState;
  printState: PrintState;
  deletedAt: string | null;
  items: TransactionItem[];
}

export interface TransactionDraftLine {
  package: RentalPackage;
  quantity: number;
}

export interface SyncConflict {
  id: string;
  transactionId: string;
  localSnapshot: Transaction;
  serverSnapshot: Transaction;
  createdAt: string;
}

export interface DashboardStats {
  gross: number;
  transactionCount: number;
  quantities: { name: string; quantity: number; accent: string }[];
  buckets: number[];
  pendingCount: number;
}
