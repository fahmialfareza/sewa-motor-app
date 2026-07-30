import {
  INVALID_SERVER_RESPONSE_MESSAGE,
  SERVER_TIMEOUT_MESSAGE,
  SERVER_UNREACHABLE_MESSAGE,
  toUserFacingErrorMessage,
} from "@/utils/errors";

describe("user-facing error messages", () => {
  it("replaces Android connection details and IP addresses", () => {
    const message = toUserFacingErrorMessage(
      new Error(
        "fetch failed: java.net.ConnectException: Failed to connect to /192.168.18.254:8080",
      ),
      "Permintaan gagal.",
    );

    expect(message).toBe(SERVER_UNREACHABLE_MESSAGE);
    expect(message).not.toMatch(/java|192\.168\.18\.254|8080/i);
  });

  it("recognizes transport error codes and timeouts", () => {
    expect(
      toUserFacingErrorMessage(
        { code: "NETWORK_UNAVAILABLE", message: "native failure" },
        "Permintaan gagal.",
      ),
    ).toBe(SERVER_UNREACHABLE_MESSAGE);
    expect(
      toUserFacingErrorMessage(
        new Error("TypeError: fetch failed because request timed out"),
        "Permintaan gagal.",
      ),
    ).toBe(SERVER_TIMEOUT_MESSAGE);
    expect(
      toUserFacingErrorMessage(
        { code: "INVALID_RESPONSE", message: "unexpected token" },
        "Permintaan gagal.",
      ),
    ).toBe(INVALID_SERVER_RESPONSE_MESSAGE);
  });

  it("preserves safe domain messages and hides technical internals", () => {
    expect(
      toUserFacingErrorMessage(
        new Error("Username sudah digunakan."),
        "Pengguna gagal disimpan.",
      ),
    ).toBe("Username sudah digunakan.");
    expect(
      toUserFacingErrorMessage(
        new Error("SQLSTATE 55000"),
        "Pengguna gagal disimpan.",
      ),
    ).toBe("Pengguna gagal disimpan.");
  });
});
