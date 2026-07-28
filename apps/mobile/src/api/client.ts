import Constants from "expo-constants";

import type { ApiEnvelope, ApiErrorEnvelope } from "./contracts";

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details: unknown;
  readonly requestId?: string | undefined;

  constructor(input: {
    status: number;
    code: string;
    message: string;
    details?: unknown;
    requestId?: string;
  }) {
    super(input.message);
    this.name = "ApiError";
    this.status = input.status;
    this.code = input.code;
    this.details = input.details;
    this.requestId = input.requestId;
  }
}

export function getApiBaseUrl(): string {
  const configured = Constants.expoConfig?.extra?.apiUrl;
  if (typeof configured !== "string" || configured.length === 0) {
    return "http://10.0.2.2:8080/api/v1";
  }
  return configured.replace(/\/$/, "");
}

export async function apiRequest<T>(
  path: string,
  options: {
    method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
    token?: string;
    body?: unknown;
    signal?: AbortSignal;
    headers?: Record<string, string>;
  } = {},
): Promise<T> {
  let response: Response;
  try {
    const request: RequestInit = {
      method: options.method ?? "GET",
      headers: {
        Accept: "application/json",
        ...(options.body === undefined
          ? {}
          : { "Content-Type": "application/json" }),
        ...(options.token ? { Authorization: `Bearer ${options.token}` } : {}),
        ...options.headers,
      },
    };
    if (options.body !== undefined) {
      request.body = JSON.stringify(options.body);
    }
    if (options.signal !== undefined) {
      request.signal = options.signal;
    }
    response = await fetch(`${getApiBaseUrl()}${path}`, request);
  } catch (error) {
    throw new ApiError({
      status: 0,
      code: "NETWORK_UNAVAILABLE",
      message:
        error instanceof Error
          ? error.message
          : "Tidak dapat terhubung ke server.",
    });
  }

  const contentType = response.headers.get("content-type");
  const payload =
    contentType?.includes("application/json") === true
      ? ((await response.json()) as ApiEnvelope<T> | ApiErrorEnvelope)
      : null;

  if (!response.ok) {
    const error =
      payload && "error" in payload
        ? payload.error
        : {
            code: `HTTP_${response.status}`,
            message: "Permintaan tidak dapat diproses.",
          };
    throw new ApiError({
      status: response.status,
      code: error.code,
      message: error.message,
      ...("details" in error ? { details: error.details } : {}),
      ...(!("requestId" in error) || error.requestId === undefined
        ? {}
        : { requestId: error.requestId }),
    });
  }

  if (!payload || !("data" in payload)) {
    throw new ApiError({
      status: response.status,
      code: "INVALID_RESPONSE",
      message: "Respons server tidak sesuai kontrak.",
    });
  }
  return payload.data;
}
