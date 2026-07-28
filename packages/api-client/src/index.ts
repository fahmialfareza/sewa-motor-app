import createClient from "openapi-fetch";

import type { components, paths } from "./generated/schema";

export type { components, operations, paths } from "./generated/schema";

export type ApiSchema = components["schemas"];
export type ApiErrorEnvelope = components["schemas"]["ErrorEnvelope"];

export interface CreateApiClientOptions {
  baseUrl: string;
  getSessionToken?: () => Promise<string | null> | string | null;
  fetch?: typeof globalThis.fetch;
}

/**
 * Creates the transport used by mobile. Domain code should call the generated
 * path methods instead of duplicating request or response interfaces.
 */
export function createApiClient(options: CreateApiClientOptions) {
  const client = createClient<paths>({
    baseUrl: options.baseUrl.replace(/\/+$/, ""),
    ...(options.fetch ? { fetch: options.fetch } : {}),
  });

  if (options.getSessionToken) {
    client.use({
      async onRequest({ request }) {
        const token = await options.getSessionToken?.();
        if (token) {
          request.headers.set("Authorization", `Bearer ${token}`);
        }
        return request;
      },
    });
  }

  return client;
}

export type ApiClient = ReturnType<typeof createApiClient>;
