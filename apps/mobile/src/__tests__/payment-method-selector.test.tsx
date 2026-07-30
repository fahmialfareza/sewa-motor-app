import { fireEvent, render } from "@testing-library/react-native";

import { PaymentMethodSelector } from "@/components/transactions/PaymentMethodSelector";

jest.mock("@/components/ui/Icon", () => ({
  Icon: () => null,
}));

describe("PaymentMethodSelector", () => {
  it("requires an explicit method and reports the selected option", () => {
    const onChange = jest.fn();
    const screen = render(
      <PaymentMethodSelector onChange={onChange} value={null} />,
    );

    expect(
      screen.getByRole("radio", { name: "Tunai" }).props.accessibilityState,
    ).toEqual({ checked: false, disabled: false });
    fireEvent.press(screen.getByRole("radio", { name: "QRIS" }));
    expect(onChange).toHaveBeenCalledWith("qris");
  });

  it("disables QRIS with an actionable configuration reason", () => {
    const onChange = jest.fn();
    const screen = render(
      <PaymentMethodSelector
        onChange={onChange}
        qrisDisabled
        qrisDisabledReason="QRIS belum dikonfigurasi."
        value={null}
      />,
    );
    const qris = screen.getByRole("radio", { name: "QRIS" });

    expect(qris.props.accessibilityState).toEqual({
      checked: false,
      disabled: true,
    });
    expect(screen.getByText("QRIS belum dikonfigurasi.")).toBeTruthy();
    fireEvent.press(qris);
    expect(onChange).not.toHaveBeenCalled();
  });
});
