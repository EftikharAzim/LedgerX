import type { Account, Entry } from "../api/endpoints";

// Default to the first active account in the displayed currency, keeping new
// travel accounts from unexpectedly changing the entry destination.
export function chooseEntryAccount(
  accounts: Account[],
  currency?: string,
): Account | undefined {
  const eligible = accounts
    .filter((a) => a.kind === "normal" && a.active)
    .sort((a, b) => a.id - b.id);
  return eligible.find((a) => a.currency === currency) ?? eligible[0];
}

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
