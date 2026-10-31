import { isValidMonth, monthLabel } from "./month.ts";

export type ReportSelection =
  | { type: "monthly"; month: string }
  | { type: "weekly"; week_of: string }
  | { type: "yearly"; year: string }
  | { type: "custom"; start_date: string; end_date: string }
  | { type: "all" };

function date(value: string): Date | null {
  if (!/^(?!0000)\d{4}-\d{2}-\d{2}$/.test(value)) return null;
  const parsed = new Date(`${value}T00:00:00Z`);
  return !Number.isNaN(parsed.getTime()) &&
    parsed.toISOString().slice(0, 10) === value
    ? parsed
    : null;
}

export function reportRange(
  selection: ReportSelection,
): { start: string | null; end: string | null } | null {
  if (selection.type === "all") return { start: null, end: null };
  if (selection.type === "monthly") {
    if (!isValidMonth(selection.month)) return null;
    const start = `${selection.month}-01`,
      end = date(start)!;
    end.setUTCMonth(end.getUTCMonth() + 1);
    end.setUTCDate(0);
    return { start, end: end.toISOString().slice(0, 10) };
  }
  if (selection.type === "yearly")
    return /^(?!0000)\d{4}$/.test(selection.year)
      ? { start: `${selection.year}-01-01`, end: `${selection.year}-12-31` }
      : null;
  if (selection.type === "weekly") {
    const start = date(selection.week_of);
    if (!start) return null;
    start.setUTCDate(start.getUTCDate() - ((start.getUTCDay() + 6) % 7));
    const end = new Date(start);
    end.setUTCDate(end.getUTCDate() + 6);
    if (start.getUTCFullYear() < 1 || end.getUTCFullYear() > 9999) return null;
    return {
      start: start.toISOString().slice(0, 10),
      end: end.toISOString().slice(0, 10),
    };
  }
  return date(selection.start_date) &&
    date(selection.end_date) &&
    selection.start_date <= selection.end_date
    ? { start: selection.start_date, end: selection.end_date }
    : null;
}

export function reportLabel(selection: ReportSelection): string {
  const range = reportRange(selection);
  if (!range) return "Choose a valid period";
  if (selection.type === "all") return "All time";
  if (selection.type === "monthly") return monthLabel(selection.month);
  if (selection.type === "yearly") return selection.year;
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeZone: "UTC",
  }).formatRange(date(range.start!)!, date(range.end!)!);
}

export function reportFilename(selection: ReportSelection): string {
  const range = reportRange(selection);
  if (!range) return "ledgerx-report.csv";
  if (selection.type === "monthly") return `ledgerx-${selection.month}.csv`;
  if (selection.type === "yearly")
    return `ledgerx-yearly-${selection.year}.csv`;
  if (selection.type === "all") return "ledgerx-all-time.csv";
  return `ledgerx-${selection.type}-${range.start}_${range.end}.csv`;
}
