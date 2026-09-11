import { ed25519 } from "@noble/curves/ed25519.js";
import * as Crypto from "expo-crypto";

import { canonicalize } from "@/utils/canonical-json";
import { activeTenantId } from "@/mode/mode-store";

import {
  bytesToHex,
  bytesToBase64,
  hexToBytes,
  readTerminalIdentity,
  writeTerminalIdentity,
  getOrCreateInstallationId,
  preserveTerminalIdentity,
  type TerminalIdentityRecord,
} from "./secure-store";

export async function getOrCreateTerminalIdentity(
  tenantId: string = activeTenantId(),
): Promise<TerminalIdentityRecord> {
  const existing = await readTerminalIdentity(tenantId);
  if (existing) return existing;

  const privateKey = await Crypto.getRandomBytesAsync(32);
  const publicKey = ed25519.getPublicKey(privateKey);
  const identity: TerminalIdentityRecord = {
    installationId: await getOrCreateInstallationId(),
    serverTerminalId: null,
    privateKeyHex: bytesToHex(privateKey),
    publicKeyHex: bytesToHex(publicKey),
    enrolledAt: null,
  };
  await writeTerminalIdentity(identity, tenantId);
  return identity;
}

export async function markTerminalEnrolled(
  serverTerminalId: string,
  tenantId: string = activeTenantId(),
): Promise<void> {
  const identity = await getOrCreateTerminalIdentity(tenantId);
  await writeTerminalIdentity(
    {
      ...identity,
      serverTerminalId,
      enrolledAt: new Date().toISOString(),
    },
    tenantId,
  );
}

export async function markTerminalRevoked(
  serverTerminalId: string,
  tenantId: string = activeTenantId(),
): Promise<void> {
  const identity = await getOrCreateTerminalIdentity(tenantId);
  if (identity.serverTerminalId !== serverTerminalId) return;
  await preserveTerminalIdentity(identity, tenantId);
  const privateKey = await Crypto.getRandomBytesAsync(32);
  const publicKey = ed25519.getPublicKey(privateKey);
  await writeTerminalIdentity(
    {
      installationId: identity.installationId,
      serverTerminalId: null,
      privateKeyHex: bytesToHex(privateKey),
      publicKeyHex: bytesToHex(publicKey),
      enrolledAt: null,
    },
    tenantId,
  );
}

export async function signCanonicalPayload(
  value: unknown,
  tenantId: string = activeTenantId(),
): Promise<string> {
  const identity = await getOrCreateTerminalIdentity(tenantId);
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

export async function getTerminalPublicKeyBase64(
  tenantId: string = activeTenantId(),
): Promise<string> {
  const identity = await getOrCreateTerminalIdentity(tenantId);
  return bytesToBase64(hexToBytes(identity.publicKeyHex));
}
