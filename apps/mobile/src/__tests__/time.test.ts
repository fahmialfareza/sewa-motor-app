import { reportingRange } from "@/utils/time";

const saturdayInJakarta = new Date("2026-07-24T20:30:00.000Z");

describe("Jakarta reporting ranges", () => {
  it("builds the local calendar day in UTC", () => {
    expect(reportingRange("daily", saturdayInJakarta)).toEqual({
      from: "2026-07-24T17:00:00.000Z",
      to: "2026-07-25T17:00:00.000Z",
    });
  });

  it("starts a week on Monday in Jakarta", () => {
    expect(reportingRange("weekly", saturdayInJakarta)).toEqual({
      from: "2026-07-19T17:00:00.000Z",
      to: "2026-07-26T17:00:00.000Z",
    });
  });

  it("uses Jakarta calendar month boundaries", () => {
    expect(reportingRange("monthly", saturdayInJakarta)).toEqual({
      from: "2026-06-30T17:00:00.000Z",
      to: "2026-07-31T17:00:00.000Z",
    });
  });
});
