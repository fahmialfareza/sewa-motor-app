const JAKARTA_OFFSET_MS = 7 * 60 * 60 * 1000;

function jakartaParts(now: Date): Date {
  return new Date(now.getTime() + JAKARTA_OFFSET_MS);
}

function asUtcFromJakarta(parts: Date): Date {
  return new Date(parts.getTime() - JAKARTA_OFFSET_MS);
}

export type ReportingPeriod = "daily" | "weekly" | "monthly";

export function reportingRange(
  period: ReportingPeriod,
  now = new Date(),
): { from: string; to: string } {
  const local = jakartaParts(now);
  const start = new Date(local);
  start.setUTCHours(0, 0, 0, 0);

  if (period === "weekly") {
    const day = start.getUTCDay();
    const fromMonday = day === 0 ? 6 : day - 1;
    start.setUTCDate(start.getUTCDate() - fromMonday);
  } else if (period === "monthly") {
    start.setUTCDate(1);
  }

  const end = new Date(start);
  if (period === "daily") end.setUTCDate(end.getUTCDate() + 1);
  if (period === "weekly") end.setUTCDate(end.getUTCDate() + 7);
  if (period === "monthly") end.setUTCMonth(end.getUTCMonth() + 1);

  return {
    from: asUtcFromJakarta(start).toISOString(),
    to: asUtcFromJakarta(end).toISOString(),
  };
}
