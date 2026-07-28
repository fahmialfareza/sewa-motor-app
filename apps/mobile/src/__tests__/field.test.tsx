import { fireEvent, render } from "@testing-library/react-native";

import { Field } from "@/components/ui/Field";

jest.mock("@/components/ui/Icon", () => ({
  Icon: () => null,
}));

describe("Field", () => {
  it("lets password fields show and hide their value", () => {
    const screen = render(
      <Field
        label="Kata sandi"
        placeholder="Masukkan kata sandi"
        secureTextEntry
      />,
    );

    expect(
      screen.getByPlaceholderText("Masukkan kata sandi").props.secureTextEntry,
    ).toBe(true);

    fireEvent.press(screen.getByLabelText("Tampilkan kata sandi"));

    expect(
      screen.getByPlaceholderText("Masukkan kata sandi").props.secureTextEntry,
    ).toBe(false);
    expect(screen.getByLabelText("Sembunyikan kata sandi")).toBeTruthy();

    fireEvent.press(screen.getByLabelText("Sembunyikan kata sandi"));

    expect(
      screen.getByPlaceholderText("Masukkan kata sandi").props.secureTextEntry,
    ).toBe(true);
  });

  it("does not render a visibility control for ordinary fields", () => {
    const screen = render(
      <Field label="Nama pengguna" placeholder="Masukkan nama pengguna" />,
    );

    expect(screen.queryByLabelText("Tampilkan kata sandi")).toBeNull();
    expect(screen.queryByLabelText("Sembunyikan kata sandi")).toBeNull();
  });

  it("keeps visibility independent between password fields", () => {
    const screen = render(
      <>
        <Field
          label="Kata sandi baru"
          placeholder="Kata sandi baru"
          secureTextEntry
        />
        <Field
          label="Konfirmasi kata sandi"
          placeholder="Konfirmasi kata sandi"
          secureTextEntry
        />
      </>,
    );

    const [firstToggle] = screen.getAllByLabelText("Tampilkan kata sandi");
    if (!firstToggle) throw new Error("Password visibility control not found.");
    fireEvent.press(firstToggle);

    expect(
      screen.getByPlaceholderText("Kata sandi baru").props.secureTextEntry,
    ).toBe(false);
    expect(
      screen.getByPlaceholderText("Konfirmasi kata sandi").props
        .secureTextEntry,
    ).toBe(true);
  });
});
