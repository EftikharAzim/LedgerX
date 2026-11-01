import type { Category } from "../api/endpoints";
import type { ReportCategory } from "../api/money-management";
import { isValidMonth } from "./month.ts";

export function shiftMonth(month: string, offset: number): string | null {
  if (!isValidMonth(month)) return null;
  const at = new Date(`${month}-01T00:00:00Z`);
  at.setUTCMonth(at.getUTCMonth() + offset);
  return at.getUTCFullYear() >= 1 && at.getUTCFullYear() <= 9999
    ? at.toISOString().slice(0, 7)
    : null;
}
export function groupReportCategories(
  rows: ReportCategory[],
  categories: Category[],
) {
  const groups = new Map<
    number,
    {
      id: number;
      name: string;
      income: number;
      spending: number;
      children: ReportCategory[];
    }
  >();
  for (const row of rows) {
    const id = row.parent_id || row.category_id;
    const group = groups.get(id) ?? {
      id,
      name: categories.find((c) => c.id === id)?.name ?? row.name,
      income: 0,
      spending: 0,
      children: [],
    };
    group.income += row.income_minor;
    group.spending += row.spending_minor;
    group.children.push(row);
    groups.set(id, group);
  }
  return [...groups.values()];
}
