import { type Entry, formatMoney } from "../api/endpoints";
import { entryKind, entryDate } from "../lib/transactions";
import Icon from "./Icon";

export default function ActivityRow({
  entry,
  category,
  accountLabel,
  onClick,
}: {
  entry: Entry;
  category?: string;
  accountLabel?: string;
  onClick: () => void;
}) {
  const kind = entryKind(entry);
  const positive = entry.amount_minor > 0;
  const amount = formatMoney(entry.amount_minor, entry.currency);
  const wideAmount = amount.length > 15;
  const title =
    entry.note ||
    (kind === "Transfer"
      ? "Account transfer"
      : kind === "Reversal"
        ? "Transaction reversal"
        : kind === "Income"
          ? "Money received"
          : kind === "Refund"
            ? "Money returned"
            : kind === "Starting balance"
              ? "Starting balance"
              : "Money spent");
  return (
    <li>
      <button
        className={`activity-row${wideAmount ? " activity-row-wide-amount" : ""}`}
        onClick={onClick}
      >
        <span className="sr-only">View </span>
        <span className="activity-description">
          <span
            className={`movement-icon ${entry.reversal_of ? "reversal" : entry.is_transfer ? "transfer" : positive ? "income" : "expense"}`}
          >
            <Icon
              name={
                entry.reversal_of
                  ? "reverse"
                  : entry.is_transfer
                    ? "transfer"
                    : positive
                      ? "down"
                      : "up"
              }
              width="18"
              height="18"
            />
          </span>
          <span>
            <strong>{title}</strong>
            <span className="entry-type">
              {kind}
              {entry.reversed_by ? " · Reversed" : ""}{" "}
              <span>· TX-{entry.transaction_id}</span>
            </span>
            {accountLabel && (
              <span className="entry-account-name">{accountLabel}</span>
            )}
          </span>
        </span>
        <span
          className={`activity-category ${!entry.category_id && entry.suggested_category_id ? "suggested" : ""}`}
        >
          {!entry.category_id && !!entry.suggested_category_id && "Suggested: "}
          {category ?? (entry.category_id ? "Categorized" : "Uncategorized")}
          {!entry.category_id && entry.suggested_category_id && (
            <span className="sr-only"> — suggested, not confirmed</span>
          )}
        </span>
        <time className="activity-date" dateTime={entry.occurred_at}>
          {entryDate(entry)}
        </time>
        <span className={`activity-amount ${positive ? "positive" : ""}`}>
          {positive ? "+" : ""}
          {amount}
          <Icon name="chevron" width="14" height="14" />
        </span>
      </button>
    </li>
  );
}
