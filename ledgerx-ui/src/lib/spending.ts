import type { ReportSelection } from "./report-period.ts";

// Report inputs describe UTC calendar dates, matching the existing CSV service.
export function defaultReportSelection(
  type: ReportSelection["type"],
  now = new Date(),
): ReportSelection {
  const date = now.toISOString().slice(0, 10);
  if (type === "monthly") return { type, month: date.slice(0, 7) };
  if (type === "weekly") return { type, week_of: date };
  if (type === "yearly") return { type, year: date.slice(0, 4) };
  if (type === "custom")
    return { type, start_date: date.slice(0, 7) + "-01", end_date: date };
  return { type: "all" };
}
