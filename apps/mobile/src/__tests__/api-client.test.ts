import { ApiError, apiRequest } from "@/api/client";
import {
  INVALID_SERVER_RESPONSE_MESSAGE,
  SERVER_UNREACHABLE_MESSAGE,
} from "@/utils/errors";

const originalFetch = globalThis.fetch;
const mockFetch = jest.fn();

describe("API client error boundary", () => {
  beforeEach(() => {
    mockFetch.mockReset();
    globalThis.fetch = mockFetch as typeof fetch;
  });

  afterAll(() => {
    globalThis.fetch = originalFetch;
  });

  it("keeps native connection details diagnostic-only", async () => {
    const nativeError = new TypeError(
      "fetch failed: java.net.ConnectException: Failed to connect to /192.168.18.254:8080",
    );
    mockFetch.mockRejectedValue(nativeError);

    const error = await apiRequest("/users").catch((reason: unknown) => reason);

    expect(error).toBeInstanceOf(ApiError);
    expect(error).toMatchObject({
      status: 0,
      code: "NETWORK_UNAVAILABLE",
      message: SERVER_UNREACHABLE_MESSAGE,
      diagnosticCause: nativeError,
    });
    expect((error as Error).message).not.toMatch(
      /java|192\.168\.18\.254|8080/i,
    );
  });

  it("translates malformed JSON without exposing parser details", async () => {
    const parserError = new SyntaxError("Unexpected token <");
    mockFetch.mockResolvedValue({
      ok: true,
      status: 200,
      headers: { get: () => "application/json" },
      json: () => Promise.reject(parserError),
    } as unknown as Response);

    await expect(apiRequest("/users")).rejects.toMatchObject({
      status: 200,
      code: "INVALID_RESPONSE",
      message: INVALID_SERVER_RESPONSE_MESSAGE,
      diagnosticCause: parserError,
    });
  });

  it("preserves backend domain messages intended for users", async () => {
    mockFetch.mockResolvedValue({
      ok: false,
      status: 422,
      headers: { get: () => "application/json" },
      json: () =>
        Promise.resolve({
          error: {
            code: "VALIDATION_ERROR",
            message: "Username sudah digunakan.",
            details: {},
            requestId: "REQUEST-1",
          },
        }),
    } as unknown as Response);

    await expect(apiRequest("/users")).rejects.toMatchObject({
      status: 422,
      code: "VALIDATION_ERROR",
      message: "Username sudah digunakan.",
      requestId: "REQUEST-1",
    });
  });
});
