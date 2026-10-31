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
    ].map((key) => qc.invalidateQueries({ queryKey: [key] })),
  );
}
