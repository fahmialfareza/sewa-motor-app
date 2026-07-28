const idrFormatter = new Intl.NumberFormat("id-ID", {
  style: "currency",
  currency: "IDR",
  maximumFractionDigits: 0,
});

const jakartaDateTimeFormatter = new Intl.DateTimeFormat("id-ID", {
  timeZone: "Asia/Jakarta",
  day: "2-digit",
  month: "short",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
});

export function formatRupiah(value: number): string {
  return idrFormatter.format(value).replace(/\u00a0/g, " ");
}

export function formatJakartaDateTime(value: string | Date): string {
  return jakartaDateTimeFormatter.format(
    typeof value === "string" ? new Date(value) : value,
  );
}

export function compactTransactionId(id: string): string {
  const displayed = displayTransactionId(id);
  if (displayed.length <= 22) return displayed;
  return `${displayed.slice(0, 13)}…${displayed.slice(-6)}`;
}

export function displayTransactionId(id: string): string {
  return id.startsWith("TRX-") ? id : `TRX-${id}`;
}

export function initials(fullName: string): string {
  return fullName
    .trim()
    .split(/\s+/)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? "")
    .join("");
}
