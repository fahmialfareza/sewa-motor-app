import * as Crypto from "expo-crypto";

import type { PaymentMethod, QrisPayloadHash } from "@/domain/types";

const STATIC_POINT_OF_INITIATION = "11";
const DYNAMIC_POINT_OF_INITIATION = "12";
const IDR_CURRENCY_CODE = "360";
const INDONESIA_COUNTRY_CODE = "ID";
const CRC_TAG_PREFIX = "6304";
const MAX_QRIS_AMOUNT = 10_000_000;
const MAX_QRIS_PAYLOAD_LENGTH = 2_048;
const QRIS_PAYLOAD_HASH_PATTERN = /^[0-9a-f]{64}$/;

const TRANSACTION_SPECIFIC_TAGS = new Set(["54", "55", "56", "57", "63"]);

type RequiredQrisTag = "00" | "01" | "52" | "53" | "58" | "59" | "60" | "63";

interface QrisElement {
  tag: string;
  value: string;
}

export interface ParsedQris {
  payload: string;
  pointOfInitiation: "static" | "dynamic";
  currency: "360";
  countryCode: "ID";
  merchantName: string;
  merchantCity: string;
  amount: string | null;
}

export interface DynamicQris extends ParsedQris {
  pointOfInitiation: "dynamic";
  amount: string;
}

function normalizePayload(value: string): string {
  const payload = value.trim();
  if (!payload) {
    throw new Error("Payload QRIS tidak boleh kosong.");
  }
  if (payload.length > MAX_QRIS_PAYLOAD_LENGTH) {
    throw new Error("Payload QRIS terlalu panjang.");
  }
  if (!/^[\x20-\x7E]+$/.test(payload)) {
    throw new Error(
      "Payload QRIS hanya boleh berisi karakter ASCII yang dapat dicetak.",
    );
  }
  return payload;
}

function parseElements(payload: string): QrisElement[] {
  const elements: QrisElement[] = [];
  let position = 0;

  while (position < payload.length) {
    if (position + 4 > payload.length) {
      throw new Error("Struktur TLV QRIS terpotong.");
    }

    const tag = payload.slice(position, position + 2);
    const encodedLength = payload.slice(position + 2, position + 4);
    if (!/^\d{2}$/.test(tag) || !/^\d{2}$/.test(encodedLength)) {
      throw new Error("Tag atau panjang TLV QRIS tidak valid.");
    }

    const length = Number.parseInt(encodedLength, 10);
    const valueStart = position + 4;
    const valueEnd = valueStart + length;
    if (valueEnd > payload.length) {
      throw new Error(`Nilai tag QRIS ${tag} terpotong.`);
    }

    elements.push({ tag, value: payload.slice(valueStart, valueEnd) });
    position = valueEnd;
  }

  return elements;
}

function validateElementCollection(elements: QrisElement[]): void {
  const seen = new Set<string>();
  let previousTag = -1;
  for (const element of elements) {
    if (seen.has(element.tag)) {
      throw new Error(`Tag QRIS ${element.tag} tidak boleh duplikat.`);
    }
    seen.add(element.tag);

    const numericTag = Number.parseInt(element.tag, 10);
    if (numericTag < previousTag) {
      throw new Error("Urutan tag QRIS tidak valid.");
    }
    previousTag = numericTag;
  }
}

function requiredElement(
  elements: QrisElement[],
  tag: RequiredQrisTag,
): QrisElement {
  const matches = elements.filter((element) => element.tag === tag);
  if (matches.length === 0) {
    throw new Error(`Tag wajib QRIS ${tag} tidak ditemukan.`);
  }
  if (matches.length > 1) {
    throw new Error(`Tag QRIS ${tag} tidak boleh duplikat.`);
  }
  const element = matches[0];
  if (!element) {
    throw new Error(`Tag wajib QRIS ${tag} tidak ditemukan.`);
  }
  return element;
}

function optionalSingletonElement(
  elements: QrisElement[],
  tag: string,
): QrisElement | null {
  const matches = elements.filter((element) => element.tag === tag);
  if (matches.length > 1) {
    throw new Error(`Tag QRIS ${tag} tidak boleh duplikat.`);
  }
  return matches[0] ?? null;
}

function encodeElement(element: QrisElement): string {
  if (element.value.length > 99) {
    throw new Error(`Nilai tag QRIS ${element.tag} terlalu panjang.`);
  }
  return `${element.tag}${String(element.value.length).padStart(2, "0")}${element.value}`;
}

export function calculateQrisCrc(value: string): string {
  let crc = 0xffff;
  for (let index = 0; index < value.length; index += 1) {
    crc ^= value.charCodeAt(index) << 8;
    for (let bit = 0; bit < 8; bit += 1) {
      crc = crc & 0x8000 ? ((crc << 1) ^ 0x1021) & 0xffff : (crc << 1) & 0xffff;
    }
  }
  return crc.toString(16).toUpperCase().padStart(4, "0");
}

export function parseQris(value: string): ParsedQris {
  const payload = normalizePayload(value);
  const elements = parseElements(payload);
  validateElementCollection(elements);
  const format = requiredElement(elements, "00");
  const initiation = requiredElement(elements, "01");
  const merchantCategory = requiredElement(elements, "52");
  const currency = requiredElement(elements, "53");
  const country = requiredElement(elements, "58");
  const merchantName = requiredElement(elements, "59");
  const merchantCity = requiredElement(elements, "60");
  const crc = requiredElement(elements, "63");
  const amount = optionalSingletonElement(elements, "54");

  if (format.value !== "01") {
    throw new Error("Versi payload QRIS tidak didukung.");
  }
  if (
    initiation.value !== STATIC_POINT_OF_INITIATION &&
    initiation.value !== DYNAMIC_POINT_OF_INITIATION
  ) {
    throw new Error(
      "Metode inisiasi QRIS harus 11 (statis) atau 12 (dinamis).",
    );
  }
  if (!/^\d{4}$/.test(merchantCategory.value)) {
    throw new Error("Merchant Category Code QRIS tidak valid.");
  }
  if (currency.value !== IDR_CURRENCY_CODE) {
    throw new Error("QRIS harus menggunakan mata uang IDR (360).");
  }
  if (country.value !== INDONESIA_COUNTRY_CODE) {
    throw new Error("QRIS harus menggunakan kode negara ID.");
  }
  if (!merchantName.value.trim() || !merchantCity.value.trim()) {
    throw new Error("Nama dan kota merchant QRIS wajib tersedia.");
  }
  if (
    !elements.some((element) => {
      const tag = Number.parseInt(element.tag, 10);
      if (tag < 26 || tag > 51 || element.value.length === 0) return false;
      const merchantElements = parseElements(element.value);
      validateElementCollection(merchantElements);
      return merchantElements.some(
        (merchantElement) =>
          merchantElement.tag === "00" && merchantElement.value.length > 0,
      );
    })
  ) {
    throw new Error("Informasi akun merchant QRIS tidak ditemukan.");
  }
  if (elements[elements.length - 1]?.tag !== "63" || crc.value.length !== 4) {
    throw new Error("CRC QRIS harus menjadi tag terakhir dengan panjang 4.");
  }

  const expectedCrc = calculateQrisCrc(payload.slice(0, -4));
  if (crc.value.toUpperCase() !== expectedCrc) {
    throw new Error("Checksum CRC QRIS tidak cocok.");
  }

  return {
    payload,
    pointOfInitiation:
      initiation.value === STATIC_POINT_OF_INITIATION ? "static" : "dynamic",
    currency: IDR_CURRENCY_CODE,
    countryCode: INDONESIA_COUNTRY_CODE,
    merchantName: merchantName.value.trim(),
    merchantCity: merchantCity.value.trim(),
    amount: amount?.value ?? null,
  };
}

export function validateStaticQris(value: string): ParsedQris {
  const parsed = parseQris(value);
  if (parsed.pointOfInitiation !== "static") {
    throw new Error("Konfigurasi harus menggunakan QRIS MPM statis.");
  }
  const transactionTag = parseElements(parsed.payload).find((element) =>
    ["54", "55", "56", "57"].includes(element.tag),
  );
  if (transactionTag) {
    throw new Error(
      `QRIS statis tidak boleh memuat tag transaksi ${transactionTag.tag}.`,
    );
  }
  return parsed;
}

export async function fingerprintStaticQris(
  value: string,
): Promise<QrisPayloadHash> {
  const payload = validateStaticQris(value).payload;
  const fingerprint = await Crypto.digestStringAsync(
    Crypto.CryptoDigestAlgorithm.SHA256,
    payload,
    { encoding: Crypto.CryptoEncoding.HEX },
  );
  const normalized = fingerprint.toLowerCase();
  if (!QRIS_PAYLOAD_HASH_PATTERN.test(normalized)) {
    throw new Error("Fingerprint QRIS merchant tidak valid.");
  }
  return normalized;
}

export function normalizeQrisPayloadHash(
  value: unknown,
): QrisPayloadHash | null {
  if (typeof value !== "string") return null;
  return QRIS_PAYLOAD_HASH_PATTERN.test(value) ? value : null;
}

export function validateQrisPayloadBinding(
  paymentMethod: PaymentMethod,
  value: QrisPayloadHash | null,
): QrisPayloadHash | null {
  if (paymentMethod !== "qris") {
    if (value !== null) {
      throw new Error(
        "Fingerprint QRIS hanya boleh disimpan untuk pembayaran QRIS.",
      );
    }
    return null;
  }

  const normalized = normalizeQrisPayloadHash(value);
  if (!normalized) {
    throw new Error(
      "Fingerprint QRIS merchant wajib tersedia untuk pembayaran QRIS.",
    );
  }
  return normalized;
}

export function validateQrisAmount(amount: number): string {
  if (
    !Number.isSafeInteger(amount) ||
    amount <= 0 ||
    amount > MAX_QRIS_AMOUNT
  ) {
    throw new Error(
      "Nominal QRIS harus berupa Rupiah bulat antara Rp1 dan Rp10.000.000.",
    );
  }
  return String(amount);
}

export function createDynamicQris(
  staticPayload: string,
  amount: number,
): DynamicQris {
  const parsed = validateStaticQris(staticPayload);
  const amountValue = validateQrisAmount(amount);
  const source = parseElements(parsed.payload);
  const converted: QrisElement[] = [];
  let amountInserted = false;

  for (const element of source) {
    if (TRANSACTION_SPECIFIC_TAGS.has(element.tag)) continue;
    if (element.tag === "01") {
      converted.push({ tag: "01", value: DYNAMIC_POINT_OF_INITIATION });
      continue;
    }
    if (element.tag === "58" && !amountInserted) {
      converted.push({ tag: "54", value: amountValue });
      amountInserted = true;
    }
    converted.push(element);
  }

  if (!amountInserted) {
    throw new Error(
      "Tag negara QRIS tidak ditemukan untuk menyisipkan nominal.",
    );
  }

  const crcInput = `${converted.map(encodeElement).join("")}${CRC_TAG_PREFIX}`;
  const payload = `${crcInput}${calculateQrisCrc(crcInput)}`;
  const validated = parseQris(payload);
  if (
    validated.pointOfInitiation !== "dynamic" ||
    validated.amount !== amountValue
  ) {
    throw new Error("QRIS dinamis gagal divalidasi.");
  }

  return {
    ...validated,
    pointOfInitiation: "dynamic",
    amount: amountValue,
  };
}
