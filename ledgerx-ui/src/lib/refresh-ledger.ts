import type { QueryClient } from "@tanstack/react-query";

export function refreshLedger(qc: QueryClient) {
  return Promise.all(
    [
      "balance",
      "summary",
      "spending",
      "activity",
      "entries",
      "welcome-activity",
      "import-context",
      "balance-checks",
      "imports",
      "categories",
    ].map((key) => qc.invalidateQueries({ queryKey: [key] })),
  );
}
