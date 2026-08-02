import { fireEvent, render } from "@testing-library/react-native";
import { Platform, Pressable, Text } from "react-native";

import { DateMonthFilter } from "@/components/reporting/DateMonthFilter";

interface MockDateTimePickerProps {
  maximumDate?: Date;
  onChange: (event: { type: "set" }, next?: Date) => void;
  timeZoneName?: string;
  value: Date;
}

let mockPickerSelection = new Date("2026-07-20T05:00:00.000Z");
const mockDateTimePicker = jest.fn((props: MockDateTimePickerProps) => (
  <Pressable
    accessibilityLabel="Mock date picker"
    accessibilityRole="button"
    onPress={() => props.onChange({ type: "set" }, mockPickerSelection)}
  >
    <Text>Mock date picker</Text>
  </Pressable>
));

jest.mock("@react-native-community/datetimepicker", () => ({
  __esModule: true,
  default: (props: MockDateTimePickerProps) => mockDateTimePicker(props),
}));

jest.mock("@/components/ui/Icon", () => ({
  Icon: () => null,
}));

describe("DateMonthFilter", () => {
  let platform: ReturnType<typeof jest.replaceProperty>;

  beforeEach(() => {
    jest.clearAllMocks();
    mockPickerSelection = new Date("2026-07-20T05:00:00.000Z");
    platform = jest.replaceProperty(Platform, "OS", "android");
  });

  afterEach(() => {
    platform.restore();
  });

  it("moves to a previous date, blocks future movement, and changes mode", () => {
    const onDateChange = jest.fn();
    const onModeChange = jest.fn();
    const screen = render(
      <DateMonthFilter
        date="2026-07-25"
        maximumDate="2026-07-25"
        mode="date"
        month="2026-07"
        onDateChange={onDateChange}
        onModeChange={onModeChange}
        onMonthChange={jest.fn()}
      />,
    );

    fireEvent.press(screen.getByRole("button", { name: "Tanggal sebelumnya" }));

    expect(onDateChange).toHaveBeenCalledWith("2026-07-24");
    expect(
      screen.getByRole("button", { name: "Tanggal berikutnya" }).props
        .accessibilityState,
    ).toEqual(expect.objectContaining({ disabled: true }));

    fireEvent.press(screen.getByRole("button", { name: "Bulan" }));
    expect(onModeChange).toHaveBeenCalledWith("month");
  });

  it("returns the Jakarta calendar key chosen with the Android date picker", () => {
    const onDateChange = jest.fn();
    const screen = render(
      <DateMonthFilter
        date="2026-07-25"
        maximumDate="2026-07-31"
        mode="date"
        month="2026-07"
        onDateChange={onDateChange}
        onModeChange={jest.fn()}
        onMonthChange={jest.fn()}
      />,
    );

    fireEvent.press(screen.getByRole("button", { name: /^Pilih tanggal\./ }));

    expect(mockDateTimePicker).toHaveBeenCalledWith(
      expect.objectContaining({
        maximumDate: new Date("2026-07-31T05:00:00.000Z"),
        timeZoneName: "Asia/Jakarta",
        value: new Date("2026-07-25T05:00:00.000Z"),
      }),
    );

    fireEvent.press(screen.getByRole("button", { name: "Mock date picker" }));

    expect(onDateChange).toHaveBeenCalledTimes(1);
    expect(onDateChange).toHaveBeenCalledWith("2026-07-20");
  });

  it("selects an available month and disables months after the maximum", () => {
    const onMonthChange = jest.fn();
    const screen = render(
      <DateMonthFilter
        date="2026-08-02"
        maximumDate="2026-08-02"
        mode="month"
        month="2026-06"
        onDateChange={jest.fn()}
        onModeChange={jest.fn()}
        onMonthChange={onMonthChange}
      />,
    );

    fireEvent.press(screen.getByRole("button", { name: /^Pilih bulan\./ }));

    expect(
      screen.getByRole("button", { name: "September 2026" }).props
        .accessibilityState,
    ).toEqual(expect.objectContaining({ disabled: true }));

    fireEvent.press(screen.getByRole("button", { name: "Agustus 2026" }));
    expect(onMonthChange).toHaveBeenCalledWith("2026-08");
  });
});
