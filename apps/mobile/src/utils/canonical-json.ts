import canonicalizeValue from "canonicalize";

export function canonicalize(value: unknown): string {
  const result = canonicalizeValue(value);
  if (result === undefined) {
    throw new TypeError("Value cannot be represented as canonical JSON");
  }
  return result;
}
