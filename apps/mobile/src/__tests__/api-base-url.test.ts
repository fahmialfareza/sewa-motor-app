import Constants from "expo-constants";

import { apiRequest, getApiBaseUrl } from "@/api/client";

jest.mock("expo-constants", () => ({
  __esModule: true,
  default: { expoConfig: { extra: { apiUrl: "" } } },
}));

const manifestExtra = Constants.expoConfig?.extra as { apiUrl: unknown };
const originalBaseUrl = process.env.EXPO_PUBLIC_API_BASE_URL;
const originalUrl = process.env.EXPO_PUBLIC_API_URL;
const originalFetch = globalThis.fetch;

describe("API base URL configuration", () => {
  beforeEach(() => {
    delete process.env["EXPO_PUBLIC_API_BASE_URL"];
    delete process.env["EXPO_PUBLIC_API_URL"];
    manifestExtra.apiUrl = "http://192.168.18.118:8080/api/v1";
  });

  afterEach(() => {
    if (originalBaseUrl === undefined) {
      delete process.env["EXPO_PUBLIC_API_BASE_URL"];
    } else {
      process.env["EXPO_PUBLIC_API_BASE_URL"] = originalBaseUrl;
    }
    if (originalUrl === undefined) {
      delete process.env["EXPO_PUBLIC_API_URL"];
    } else {
      process.env["EXPO_PUBLIC_API_URL"] = originalUrl;
    }
    globalThis.fetch = originalFetch;
  });

  it("uses the bundle environment instead of a stale native manifest", () => {
    process.env["EXPO_PUBLIC_API_URL"] = "http://192.168.18.118:8000/api/v1";

    expect(getApiBaseUrl()).toBe("http://192.168.18.118:8000/api/v1");
  });

  it("preserves the explicit API_BASE_URL alias priority", () => {
    process.env["EXPO_PUBLIC_API_BASE_URL"] = "https://api.example.com/api/v1";
    process.env["EXPO_PUBLIC_API_URL"] = "http://192.168.18.118:8000/api/v1";

    expect(getApiBaseUrl()).toBe("https://api.example.com/api/v1");
  });

  it("ignores blank overrides and normalizes the configured URL", () => {
    process.env["EXPO_PUBLIC_API_BASE_URL"] = " ";
    process.env["EXPO_PUBLIC_API_URL"] = " http://192.168.18.118:8000/api/v1/ ";

    expect(getApiBaseUrl()).toBe("http://192.168.18.118:8000/api/v1");
  });

  it("retains manifest-only configuration for compatible builds", () => {
    process.env["EXPO_PUBLIC_API_URL"] = "";
    manifestExtra.apiUrl = " https://api.example.com/api/v1/ ";

    expect(getApiBaseUrl()).toBe("https://api.example.com/api/v1");
  });

  it.each([undefined, null, 123, "", " "])(
    "uses the port 8000 emulator default when no URL is configured (%p)",
    (configured) => {
      manifestExtra.apiUrl = configured;

      expect(getApiBaseUrl()).toBe("http://10.0.2.2:8000/api/v1");
    },
  );

  it("sends requests to port 8000 even when the APK still embeds port 8080", async () => {
    process.env["EXPO_PUBLIC_API_URL"] = "http://192.168.18.118:8000/api/v1";
    const mockFetch = jest.fn().mockResolvedValue({
      ok: true,
      status: 200,
      headers: { get: () => "application/json" },
      json: async () => ({ data: { ready: true } }),
    });
    globalThis.fetch = mockFetch;

    await expect(apiRequest("/health")).resolves.toEqual({ ready: true });
    expect(mockFetch).toHaveBeenCalledWith(
      "http://192.168.18.118:8000/api/v1/health",
      expect.objectContaining({ method: "GET" }),
    );
  });
});
