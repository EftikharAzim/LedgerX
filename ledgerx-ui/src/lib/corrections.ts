import type { CorrectionDraft } from "../api/workspace.ts";
import { parseAmount, parseStartingBalance, toOccurredAt } from "./money.ts";

export type EditKind =
  | "expense"
  | "income"
  | "refund"
  | "transfer"
  | "opening_balance";
export function correctionKind(draft: CorrectionDraft): EditKind {
  return draft.entry_type === "standard"
    ? draft.amount_minor < 0
      ? "expense"
      : "income"
    : draft.entry_type;
}

// The source instant stays exact, including seconds, unless the date/time is edited.
export function correctionDraft(
  original: CorrectionDraft,
  input: {
    kind: EditKind;
    amount: string;
    accountID: number;
    toAccountID: number;
    categoryID: number;
    note: string;
    dateOnly: boolean;
    date: string;
    localTime: string;
    timeEdited: boolean;
  },
): CorrectionDraft | null {
  const amount =
    input.kind === "opening_balance"
      ? parseStartingBalance(input.amount, original.currency)
      : parseAmount(input.amount, original.currency);
  const occurred = input.dateOnly
    ? /^\d{4}-\d{2}-\d{2}$/.test(input.date)
      ? input.date + "T00:00:00Z"
      : null
    : !input.timeEdited && !original.date_only
      ? original.occurred_at
      : toOccurredAt(input.localTime);
  if (
    amount === null ||
    !occurred ||
    !input.accountID ||
    (input.kind === "transfer" &&
      (!input.toAccountID || input.toAccountID === input.accountID))
  )
    return null;
  return {
    account_id: input.accountID,
    ...(input.kind === "transfer" ? { to_account_id: input.toAccountID } : {}),
    amount_minor:
      input.kind === "expense" || input.kind === "transfer" ? -amount : amount,
    currency: original.currency,
    entry_type:
      input.kind === "income" || input.kind === "expense"
        ? "standard"
        : input.kind,
    occurred_at: occurred,
    date_only: input.dateOnly,
    note: input.note.trim(),
    category_id:
      input.kind === "transfer" || input.kind === "opening_balance"
        ? 0
        : input.categoryID,
  };
}

export function sameCorrection(
  a: CorrectionDraft,
  b: CorrectionDraft,
): boolean {
  return (
    a.account_id === b.account_id &&
    (a.to_account_id || 0) === (b.to_account_id || 0) &&
    a.amount_minor === b.amount_minor &&
    a.currency === b.currency &&
    a.entry_type === b.entry_type &&
    new Date(a.occurred_at).getTime() === new Date(b.occurred_at).getTime() &&
    a.date_only === b.date_only &&
    a.note === b.note &&
    a.category_id === b.category_id
  );
}
