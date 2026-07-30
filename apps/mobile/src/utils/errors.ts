export const SERVER_UNREACHABLE_MESSAGE =
  "Server belum dapat dijangkau. Pastikan perangkat terhubung ke jaringan Wi-Fi yang benar dan server sedang aktif, lalu coba lagi.";

export const SERVER_TIMEOUT_MESSAGE =
  "Server terlalu lama merespons. Periksa koneksi jaringan, lalu coba lagi.";

export const INVALID_SERVER_RESPONSE_MESSAGE =
  "Respons server tidak dapat dibaca. Coba lagi atau hubungi pengelola sistem.";

const NETWORK_ERROR_PATTERN =
  /fetch failed|network request failed|network error|failed to connect|connectexception|connection refused|econnrefused|enetunreach|ehostunreach|unknownhostexception|unable to resolve host|java\.net/i;
const TIMEOUT_ERROR_PATTERN =
  /request timed out|timed out|timeout|etimedout|sockettimeoutexception/i;
const TECHNICAL_ERROR_PATTERN =
  /sqlstate|sqlite|gorm|pgx|typeerror:|undefined is not a function|java\.(?:io|lang)\./i;

interface ErrorLike {
  code?: unknown;
  message?: unknown;
}

function errorLike(reason: unknown): ErrorLike | null {
  if (reason instanceof Error) return reason as Error & ErrorLike;
  if (reason && typeof reason === "object") return reason as ErrorLike;
  return null;
}

export function toUserFacingErrorMessage(
  reason: unknown,
  fallback: string,
): string {
  const value = errorLike(reason);
  const code = typeof value?.code === "string" ? value.code : null;
  const message =
    typeof reason === "string"
      ? reason.trim()
      : typeof value?.message === "string"
        ? value.message.trim()
        : "";

  if (code === "REQUEST_TIMEOUT" || TIMEOUT_ERROR_PATTERN.test(message)) {
    return SERVER_TIMEOUT_MESSAGE;
  }
  if (code === "NETWORK_UNAVAILABLE" || NETWORK_ERROR_PATTERN.test(message)) {
    return SERVER_UNREACHABLE_MESSAGE;
  }
  if (code === "INVALID_RESPONSE") {
    return INVALID_SERVER_RESPONSE_MESSAGE;
  }
  if (!message || TECHNICAL_ERROR_PATTERN.test(message)) return fallback;
  return message;
}
