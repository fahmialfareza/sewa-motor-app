import { fireEvent, render } from "@testing-library/react-native";

import { DynamicQrisCard } from "@/components/payments/DynamicQrisCard";

interface MockQrCodeProps {
  value: string;
  [key: string]: unknown;
}

const mockQrCode = jest.fn((_props: MockQrCodeProps) => null);

jest.mock("react-native-qrcode-svg", () => ({
  __esModule: true,
  default: (props: MockQrCodeProps) => mockQrCode(props),
}));

jest.mock("@/components/ui/Icon", () => ({
  Icon: () => null,
}));

describe("DynamicQrisCard", () => {
  beforeEach(() => {
    mockQrCode.mockClear();
  });

  it("renders the exact payment payload with accessible amount and merchant details", () => {
    const payload =
      "00020101021226670016COM.NOBUBANK.WWW01189360050300000879140214567890123456780303UMI51440014ID.CO.QRIS.WWW0215ID10200123456780303UMI52045411530336054061255005802ID5919SEWA MOTOR BAHAGIA6008DENPASAR6304ABCD";
    const screen = render(
      <DynamicQrisCard
        amount={125_500}
        error={null}
        merchantCity="DENPASAR"
        merchantName="SEWA MOTOR BAHAGIA"
        payload={payload}
      />,
    );

    expect(screen.getByText("Rp 125.500")).toBeTruthy();
    expect(screen.getByText("SEWA MOTOR BAHAGIA")).toBeTruthy();
    expect(screen.getByText("DENPASAR")).toBeTruthy();
    const accessibleQr = screen.getByLabelText(
      "QRIS pembayaran Rp 125.500 untuk SEWA MOTOR BAHAGIA",
    );
    expect(accessibleQr.props.accessibilityRole).toBe("image");
    expect(mockQrCode).toHaveBeenCalledTimes(1);
    expect(mockQrCode.mock.calls[0]?.[0]).toMatchObject({
      value: payload,
      ecl: "M",
      size: 240,
      quietZone: 32,
      color: "#000000",
      backgroundColor: "#FFFFFF",
    });
    expect(mockQrCode.mock.calls[0]?.[0].enableLinearGradient).not.toBe(true);
  });

  it("shows an accessible error and lets the operator configure QRIS", () => {
    const onConfigure = jest.fn();
    const error = "QRIS statis belum dikonfigurasi.";
    const screen = render(
      <DynamicQrisCard
        amount={125_500}
        error={error}
        merchantCity={null}
        merchantName={null}
        onConfigure={onConfigure}
        payload={null}
      />,
    );

    expect(screen.getByRole("alert").props.children).toBe(error);
    const configureButton = screen.getByRole("button", {
      name: "Atur QRIS merchant",
    });
    fireEvent.press(configureButton);

    expect(onConfigure).toHaveBeenCalledTimes(1);
    expect(mockQrCode).not.toHaveBeenCalled();
  });

  it("does not render a configuration action without a callback", () => {
    const screen = render(
      <DynamicQrisCard
        amount={125_500}
        error="Payload QRIS tidak dapat dibuat."
        merchantCity="DENPASAR"
        merchantName="SEWA MOTOR BAHAGIA"
        payload={null}
      />,
    );

    expect(screen.getByText("Payload QRIS tidak dapat dibuat.")).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: "Atur QRIS merchant" }),
    ).toBeNull();
    expect(mockQrCode).not.toHaveBeenCalled();
  });
});
