import {
  calendarDateForPicker,
  calendarDateFromPicker,
  calendarDateKey,
  calendarMonthKey,
  currentJakartaDate,
  currentJakartaMonth,
  daysInCalendarMonth,
  monthFromCalendarDate,
  normalizeUtcTimestamp,
  parseCalendarDateKey,
  parseCalendarMonthKey,
  reportingRange,
  shiftReportingSelection,
} from "@/utils/time";

describe("Jakarta reporting ranges", () => {
  it("builds an explicitly selected past calendar day in UTC", () => {
    expect(reportingRange("date", "2026-07-25")).toEqual({
      mode: "date",
      from: "2026-07-24T17:00:00.000Z",
      to: "2026-07-25T17:00:00.000Z",
    });
  });

  it("builds an explicitly selected calendar month in UTC", () => {
    expect(reportingRange("month", "2026-07")).toEqual({
      mode: "month",
      from: "2026-06-30T17:00:00.000Z",
      to: "2026-07-31T17:00:00.000Z",
    });
  });

  it("includes all 29 days of a leap-year February", () => {
    expect(daysInCalendarMonth("2028-02")).toBe(29);
    expect(daysInCalendarMonth("2027-02")).toBe(28);
    expect(reportingRange("month", "2028-02")).toEqual({
      mode: "month",
      from: "2028-01-31T17:00:00.000Z",
      to: "2028-02-29T17:00:00.000Z",
    });
  });
});

describe("Jakarta calendar selections", () => {
  it("moves calendar dates across the December and January boundary", () => {
    expect(shiftReportingSelection("date", "2026-12-31", 1)).toBe("2027-01-01");
    expect(shiftReportingSelection("date", "2027-01-01", -1)).toBe(
      "2026-12-31",
    );
  });

  it("moves calendar months across the December and January boundary", () => {
    expect(shiftReportingSelection("month", "2026-12", 1)).toBe("2027-01");
    expect(shiftReportingSelection("month", "2027-01", -1)).toBe("2026-12");
  });

  it("changes the current Jakarta date at exactly Jakarta midnight", () => {
    expect(currentJakartaDate(new Date("2026-07-29T16:59:59.999Z"))).toBe(
      "2026-07-29",
    );
    expect(currentJakartaDate(new Date("2026-07-29T17:00:00.000Z"))).toBe(
      "2026-07-30",
    );
  });

  it("changes the current Jakarta month at its local month boundary", () => {
    expect(currentJakartaMonth(new Date("2026-07-31T16:59:59.999Z"))).toBe(
      "2026-07",
    );
    expect(currentJakartaMonth(new Date("2026-07-31T17:00:00.000Z"))).toBe(
      "2026-08",
    );
  });

  it("creates canonical calendar keys and derives their month", () => {
    expect(calendarDateKey(2028, 2, 29)).toBe("2028-02-29");
    expect(calendarMonthKey(2026, 7)).toBe("2026-07");
    expect(monthFromCalendarDate("2028-02-29")).toBe("2028-02");
  });

  it("rejects malformed or impossible calendar selections", () => {
    expect(() => calendarDateKey(2027, 2, 29)).toThrow(
      "Tanggal kalender tidak valid.",
    );
    expect(() => parseCalendarDateKey("2026-7-25")).toThrow(
      "Tanggal kalender tidak valid.",
    );
    expect(() => parseCalendarMonthKey("2026-13")).toThrow(
      "Bulan kalender tidak valid.",
    );
    expect(() => shiftReportingSelection("date", "2026-07-25", 0.5)).toThrow(
      "Perpindahan periode tidak valid.",
    );
  });
});

describe("date picker conversion", () => {
  it("round-trips a calendar date through a safe Jakarta-noon instant", () => {
    const pickerDate = calendarDateForPicker("2026-07-25");

    expect(pickerDate.toISOString()).toBe("2026-07-25T05:00:00.000Z");
    expect(calendarDateFromPicker(pickerDate)).toBe("2026-07-25");
  });

  it("reads the Jakarta calendar date from a picker instant", () => {
    expect(calendarDateFromPicker(new Date("2026-07-24T17:00:00.000Z"))).toBe(
      "2026-07-25",
    );
  });
});

describe("UTC timestamp normalization", () => {
  it("canonicalizes a Jakarta offset timestamp without changing its instant", () => {
    expect(normalizeUtcTimestamp("2026-07-29T18:00:00+07:00")).toBe(
      "2026-07-29T11:00:00.000Z",
    );
  });

  it("rejects invalid API timestamps before they reach SQLite", () => {
    expect(() => normalizeUtcTimestamp("not-a-timestamp")).toThrow(
      "Timestamp tidak valid.",
    );
  });
});
