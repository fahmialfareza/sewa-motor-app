import { ed25519 } from "@noble/curves/ed25519.js";
import * as Crypto from "expo-crypto";

import { canonicalize } from "@/utils/canonical-json";

import {
  bytesToHex,
  bytesToBase64,
  hexToBytes,
  readTerminalIdentity,
  writeTerminalIdentity,
  type TerminalIdentityRecord,
} from "./secure-store";

export async function getOrCreateTerminalIdentity(): Promise<TerminalIdentityRecord> {
  const existing = await readTerminalIdentity();
  if (existing) return existing;

  const privateKey = await Crypto.getRandomBytesAsync(32);
  const publicKey = ed25519.getPublicKey(privateKey);
  const identity: TerminalIdentityRecord = {
    installationId: Crypto.randomUUID(),
    serverTerminalId: null,
    privateKeyHex: bytesToHex(privateKey),
    publicKeyHex: bytesToHex(publicKey),
    enrolledAt: null,
  };
  await writeTerminalIdentity(identity);
  return identity;
}

export async function markTerminalEnrolled(
  serverTerminalId: string,
): Promise<void> {
  const identity = await getOrCreateTerminalIdentity();
  await writeTerminalIdentity({
    ...identity,
    serverTerminalId,
    enrolledAt: new Date().toISOString(),
  });
}

export async function markTerminalRevoked(
  serverTerminalId: string,
): Promise<void> {
  const identity = await getOrCreateTerminalIdentity();
  if (identity.serverTerminalId !== serverTerminalId) return;
  const privateKey = await Crypto.getRandomBytesAsync(32);
  const publicKey = ed25519.getPublicKey(privateKey);
  await writeTerminalIdentity({
    installationId: Crypto.randomUUID(),
    serverTerminalId: null,
    privateKeyHex: bytesToHex(privateKey),
    publicKeyHex: bytesToHex(publicKey),
    enrolledAt: null,
  });
}

export async function signCanonicalPayload(value: unknown): Promise<string> {
  const identity = await getOrCreateTerminalIdentity();
  return signCanonicalPayloadWithKey(value, hexToBytes(identity.privateKeyHex));
}

export function signCanonicalPayloadWithKey(
  value: unknown,
  privateKey: Uint8Array,
): string {
  const message = new TextEncoder().encode(canonicalize(value));
  const signature = ed25519.sign(message, privateKey);
  return bytesToBase64(signature);
}

export async function getTerminalPublicKeyBase64(): Promise<string> {
  const identity = await getOrCreateTerminalIdentity();
  return bytesToBase64(hexToBytes(identity.publicKeyHex));
}
