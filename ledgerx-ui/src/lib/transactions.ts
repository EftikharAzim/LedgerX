import type { Entry } from "../api/endpoints";

export function entryKind(
  entry: Pick<
    Entry,
    "amount_minor" | "reversal_of" | "is_transfer" | "entry_type"
  >,
) {
  if (entry.reversal_of) return "Reversal";
  if (entry.entry_type === "opening_balance") return "Starting balance";
  if (entry.entry_type === "refund") return "Refund";
  if (entry.is_transfer) return "Transfer";
  return entry.amount_minor > 0 ? "Income" : "Expense";
}
