const JAKARTA_OFFSET_MS = 7 * 60 * 60 * 1000;
const DATE_KEY_PATTERN = /^(\d{4})-(\d{2})-(\d{2})$/;
const MONTH_KEY_PATTERN = /^(\d{4})-(\d{2})$/;

export type ReportingMode = "date" | "month";
export type CalendarDateKey = string;
export type CalendarMonthKey = string;

export interface ReportingRange {
  mode: ReportingMode;
  from: string;
  to: string;
}

interface CalendarDateParts {
  year: number;
  month: number;
  day: number;
}

interface CalendarMonthParts {
  year: number;
  month: number;
}

function pad(value: number): string {
  return String(value).padStart(2, "0");
}

function validateMonth(year: number, month: number): void {
  if (!Number.isInteger(year) || year < 1900 || year > 9999) {
    throw new Error("Tahun kalender tidak valid.");
  }
  if (!Number.isInteger(month) || month < 1 || month > 12) {
    throw new Error("Bulan kalender tidak valid.");
  }
}

export function calendarMonthKey(
  year: number,
  month: number,
): CalendarMonthKey {
  validateMonth(year, month);
  return `${String(year).padStart(4, "0")}-${pad(month)}`;
}

export function calendarDateKey(
  year: number,
  month: number,
  day: number,
): CalendarDateKey {
  validateMonth(year, month);
  if (!Number.isInteger(day) || day < 1 || day > 31) {
    throw new Error("Tanggal kalender tidak valid.");
  }

  const candidate = new Date(Date.UTC(year, month - 1, day));
  if (
    candidate.getUTCFullYear() !== year ||
    candidate.getUTCMonth() !== month - 1 ||
    candidate.getUTCDate() !== day
  ) {
    throw new Error("Tanggal kalender tidak valid.");
  }
  return `${calendarMonthKey(year, month)}-${pad(day)}`;
}

export function parseCalendarDateKey(value: string): CalendarDateParts {
  const match = DATE_KEY_PATTERN.exec(value);
  if (!match) throw new Error("Tanggal kalender tidak valid.");

  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  calendarDateKey(year, month, day);
  return { year, month, day };
}

export function parseCalendarMonthKey(value: string): CalendarMonthParts {
  const match = MONTH_KEY_PATTERN.exec(value);
  if (!match) throw new Error("Bulan kalender tidak valid.");

  const year = Number(match[1]);
  const month = Number(match[2]);
  validateMonth(year, month);
  return { year, month };
}

export function currentJakartaDate(now = new Date()): CalendarDateKey {
  const local = new Date(now.getTime() + JAKARTA_OFFSET_MS);
  return calendarDateKey(
    local.getUTCFullYear(),
    local.getUTCMonth() + 1,
    local.getUTCDate(),
  );
}

export function currentJakartaMonth(now = new Date()): CalendarMonthKey {
  return currentJakartaDate(now).slice(0, 7);
}

export function monthFromCalendarDate(date: CalendarDateKey): CalendarMonthKey {
  parseCalendarDateKey(date);
  return date.slice(0, 7);
}

export function calendarDateForPicker(date: CalendarDateKey): Date {
  const { year, month, day } = parseCalendarDateKey(date);
  // 05:00 UTC is noon in Jakarta, safely away from either calendar boundary.
  return new Date(Date.UTC(year, month - 1, day, 5));
}

export function calendarDateFromPicker(date: Date): CalendarDateKey {
  return currentJakartaDate(date);
}

export function daysInCalendarMonth(monthKey: CalendarMonthKey): number {
  const { year, month } = parseCalendarMonthKey(monthKey);
  return new Date(Date.UTC(year, month, 0)).getUTCDate();
}

export function shiftReportingSelection(
  mode: "date",
  selection: CalendarDateKey,
  amount: number,
): CalendarDateKey;
export function shiftReportingSelection(
  mode: "month",
  selection: CalendarMonthKey,
  amount: number,
): CalendarMonthKey;
export function shiftReportingSelection(
  mode: ReportingMode,
  selection: string,
  amount: number,
): string {
  if (!Number.isInteger(amount)) {
    throw new Error("Perpindahan periode tidak valid.");
  }

  if (mode === "date") {
    const { year, month, day } = parseCalendarDateKey(selection);
    const shifted = new Date(Date.UTC(year, month - 1, day + amount));
    return calendarDateKey(
      shifted.getUTCFullYear(),
      shifted.getUTCMonth() + 1,
      shifted.getUTCDate(),
    );
  }

  const { year, month } = parseCalendarMonthKey(selection);
  const shifted = new Date(Date.UTC(year, month - 1 + amount, 1));
  return calendarMonthKey(shifted.getUTCFullYear(), shifted.getUTCMonth() + 1);
}

export function reportingRange(
  mode: "date",
  selection: CalendarDateKey,
): ReportingRange;
export function reportingRange(
  mode: "month",
  selection: CalendarMonthKey,
): ReportingRange;
export function reportingRange(
  mode: ReportingMode,
  selection: string,
): ReportingRange {
  let startUtc: number;
  let endUtc: number;

  if (mode === "date") {
    const { year, month, day } = parseCalendarDateKey(selection);
    startUtc = Date.UTC(year, month - 1, day) - JAKARTA_OFFSET_MS;
    endUtc = Date.UTC(year, month - 1, day + 1) - JAKARTA_OFFSET_MS;
  } else {
    const { year, month } = parseCalendarMonthKey(selection);
    startUtc = Date.UTC(year, month - 1, 1) - JAKARTA_OFFSET_MS;
    endUtc = Date.UTC(year, month, 1) - JAKARTA_OFFSET_MS;
  }

  return {
    mode,
    from: new Date(startUtc).toISOString(),
    to: new Date(endUtc).toISOString(),
  };
}

export function normalizeUtcTimestamp(value: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    throw new Error("Timestamp tidak valid.");
  }
  return parsed.toISOString();
}
