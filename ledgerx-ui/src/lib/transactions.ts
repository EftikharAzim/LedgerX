import type { Entry } from "../api/endpoints";

export function entryKind(
  entry: Pick<Entry, "amount_minor" | "reversal_of" | "is_transfer">,
) {
  if (entry.reversal_of) return "Reversal";
  if (entry.is_transfer) return "Transfer";
  return entry.amount_minor > 0 ? "Income" : "Expense";
}
