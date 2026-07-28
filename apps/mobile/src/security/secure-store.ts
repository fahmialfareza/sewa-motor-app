import * as Crypto from "expo-crypto";
import * as SecureStore from "expo-secure-store";

import type { Session } from "@/domain/types";

const keys = {
  session: "sewa-motor.session.v1",
  database: "sewa-motor.database-key.v1",
  terminal: "sewa-motor.terminal-identity.v1",
  printer: "sewa-motor.printer-config.v1",
} as const;

export interface TerminalIdentityRecord {
  installationId: string;
  serverTerminalId: string | null;
  privateKeyHex: string;
  publicKeyHex: string;
  enrolledAt: string | null;
}

export interface PrinterConfig {
  adapter: "simulator" | "bluetooth" | "integrated";
  address: string | null;
  displayName: string;
  paperColumns: 32 | 48;
}

const secureOptions: SecureStore.SecureStoreOptions = {
  keychainAccessible: SecureStore.WHEN_UNLOCKED_THIS_DEVICE_ONLY,
};

export async function readSession(): Promise<Session | null> {
  return readJson<Session>(keys.session);
}

export async function writeSession(session: Session): Promise<void> {
  await writeJson(keys.session, session);
}

export async function clearSession(): Promise<void> {
  await SecureStore.deleteItemAsync(keys.session);
}

export async function getOrCreateDatabaseKey(): Promise<string> {
  const existing = await SecureStore.getItemAsync(keys.database);
  if (existing) return existing;
  const generated = bytesToHex(await Crypto.getRandomBytesAsync(32));
  await SecureStore.setItemAsync(keys.database, generated, secureOptions);
  return generated;
}

export async function readTerminalIdentity(): Promise<TerminalIdentityRecord | null> {
  const value = await readJson<
    TerminalIdentityRecord & { terminalId?: string }
  >(keys.terminal);
  if (!value) return null;
  if (value.installationId) return value;
  return {
    installationId: value.terminalId ?? "unknown-installation",
    serverTerminalId: null,
    privateKeyHex: value.privateKeyHex,
    publicKeyHex: value.publicKeyHex,
    enrolledAt: null,
  };
}

export async function writeTerminalIdentity(
  identity: TerminalIdentityRecord,
): Promise<void> {
  await writeJson(keys.terminal, identity);
}

export async function readPrinterConfig(): Promise<PrinterConfig> {
  return (
    (await readJson<PrinterConfig>(keys.printer)) ?? {
      adapter: "simulator",
      address: null,
      displayName: "Simulator printer",
      paperColumns: 32,
    }
  );
}

export async function writePrinterConfig(config: PrinterConfig): Promise<void> {
  await writeJson(keys.printer, config);
}

async function readJson<T>(key: string): Promise<T | null> {
  const value = await SecureStore.getItemAsync(key);
  if (!value) return null;
  try {
    return JSON.parse(value) as T;
  } catch {
    await SecureStore.deleteItemAsync(key);
    return null;
  }
}

async function writeJson(key: string, value: unknown): Promise<void> {
  await SecureStore.setItemAsync(key, JSON.stringify(value), secureOptions);
}

export function bytesToHex(bytes: Uint8Array): string {
  return Array.from(bytes, (value) => value.toString(16).padStart(2, "0")).join(
    "",
  );
}

export function hexToBytes(hex: string): Uint8Array {
  if (hex.length % 2 !== 0) throw new Error("Invalid hex value");
  const values = hex.match(/.{2}/g) ?? [];
  return Uint8Array.from(values.map((value) => Number.parseInt(value, 16)));
}

export function bytesToBase64(bytes: Uint8Array): string {
  const alphabet =
    "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
  let output = "";
  for (let index = 0; index < bytes.length; index += 3) {
    const first = bytes[index] ?? 0;
    const second = bytes[index + 1];
    const third = bytes[index + 2];
    const chunk = (first << 16) | ((second ?? 0) << 8) | (third ?? 0);
    output += alphabet[(chunk >> 18) & 63];
    output += alphabet[(chunk >> 12) & 63];
    output += second === undefined ? "=" : alphabet[(chunk >> 6) & 63];
    output += third === undefined ? "=" : alphabet[chunk & 63];
  }
  return output;
}
