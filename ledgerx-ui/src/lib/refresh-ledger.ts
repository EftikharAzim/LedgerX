import type { QueryClient } from "@tanstack/react-query";

export function refreshLedger(qc: QueryClient) {
  return Promise.all(
    [
      "plans",
      "schedules",
      "cashflow",
      "financial-report",
      "budgets",
      "category-impact",
      "balance",
      "summary",
      "spending",
      "activity",
      "entries",
      "welcome-activity",
      "import-context",
      "balance-checks",
      "imports",
      "workspace",
      "edit-context",
      "categories",
    ].map((key) => qc.invalidateQueries({ queryKey: [key] })),
  );
}
