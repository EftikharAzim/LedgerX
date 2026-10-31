import { Link, useParams } from "react-router-dom";
import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useState } from "react";
import {
  type Entry,
  formatMoney,
  getCurrentBalance,
  getMonthlySummary,
  listAccounts,
  listCategories,
  listEntries,
} from "../api/endpoints";
import { apiError } from "../lib/errors";
import { readAppearance } from "../lib/account-appearance";
import { entryKind } from "../lib/transactions";
import Icon from "../components/Icon";
import TransactionDialog from "../components/TransactionDialog";
import TransferDialog from "../components/TransferDialog";
import TransactionDrawer from "../components/TransactionDrawer";
import CategoryManager from "../components/CategoryManager";

export default function AccountDetails() {
  const { id } = useParams();
  const accountId = Number(id);
  if (!Number.isSafeInteger(accountId) || accountId <= 0)
    return <AccountNotFound />;
  return <AccountWorkspace key={accountId} accountId={accountId} />;
}

function AccountWorkspace({ accountId }: { accountId: number }) {
  const qc = useQueryClient();
  const [ym, setYm] = useState(() => new Date().toISOString().slice(0, 7));
  const [dialog, setDialog] = useState<"transaction" | "transfer" | null>(null);
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [message, setMessage] = useState("");
  const accountsQuery = useQuery({
    queryKey: ["accounts"],
    queryFn: listAccounts,
  });
  const account = accountsQuery.data?.find(
    (a) => a.id === accountId && a.kind === "normal",
  );
  const summary = useQuery({
    queryKey: ["summary", accountId, ym],
    queryFn: () => getMonthlySummary(accountId, ym),
    enabled: !!account,
  });
  const balance = useQuery({
    queryKey: ["balance", accountId],
    queryFn: () => getCurrentBalance(accountId),
    enabled: !!account,
  });
  const entries = useInfiniteQuery({
    queryKey: ["entries", accountId],
    queryFn: ({ pageParam }) => listEntries(accountId, pageParam || undefined),
    initialPageParam: "",
    getNextPageParam: (last) => last.next_cursor || undefined,
    enabled: !!account,
  });
  const categories = useQuery({
    queryKey: ["categories"],
    queryFn: listCategories,
    enabled: !!account,
  });
  const allEntries = entries.data?.pages.flatMap((page) => page.entries) ?? [];
  const selected = allEntries.find((e) => e.posting_id === selectedId);
  const refreshEntries = () => qc.invalidateQueries({ queryKey: ["entries"] });
  const onSaved = (notice: string) => {
    setDialog(null);
    setSelectedId(null);
    setMessage(notice);
    void Promise.all([
      refreshEntries(),
      qc.invalidateQueries({ queryKey: ["balance"] }),
      qc.invalidateQueries({ queryKey: ["summary"] }),
    ]);
  };
  const open = (kind: "transaction" | "transfer") => {
    setMessage("");
    setDialog(kind);
  };

  if (accountsQuery.isPending)
    return (
      <div
        className="detail-loading"
        aria-busy="true"
        aria-label="Loading account"
      >
        <span className="skeleton skeleton-line" />
        <span className="skeleton" />
        <span className="skeleton" />
      </div>
    );
  if (accountsQuery.isError)
    return (
      <div className="error-notice" role="alert">
        <span>
          {apiError(accountsQuery.error, "Couldn't load this account.")}
        </span>
        <button className="text-button" onClick={() => accountsQuery.refetch()}>
          Try again
        </button>
      </div>
    );
  if (!account) return <AccountNotFound />;
  const appearance = readAppearance(account.id);
  const relatedId = selected?.reversal_of ?? selected?.reversed_by;
  const monthLabel = new Date(`${ym}-01T00:00:00Z`).toLocaleDateString(
    undefined,
    { month: "long", year: "numeric", timeZone: "UTC" },
  );

  return (
    <div className="account-details-page">
      <Link to="/accounts" className="back-link">
        <Icon name="arrow" width="15" height="15" />
        All accounts
      </Link>
      <div className="detail-page-heading">
        <div className="detail-account-title">
          <span className={`account-symbol tone-${appearance.color}`}>
            <Icon name={appearance.icon} width="27" height="27" />
          </span>
          <div>
            <p className="eyebrow">A CLOSER LOOK</p>
            <h1>{account.name}</h1>
            <p className="detail-account-meta">
              <span className="account-currency">{account.currency}</span>
              <span
                className={account.active ? "active-dot" : "inactive-dot"}
              />
              {account.active ? "Active account" : "Inactive account"}
            </p>
          </div>
        </div>
        <div className="detail-actions">
          <button
            className="btn"
            onClick={() => open("transfer")}
            disabled={!account.active}
          >
            <Icon name="transfer" width="17" height="17" />
            Transfer
          </button>
          <button
            className="btn btn-primary"
            onClick={() => open("transaction")}
            disabled={!account.active}
          >
            <Icon name="plus" width="17" height="17" />
            Add transaction
          </button>
        </div>
      </div>
      <div
        role="status"
        aria-live="polite"
        className={message ? "success-notice" : "sr-only"}
      >
        {message && (
          <>
            <Icon name="check" />
            {message}
          </>
        )}
      </div>
      {!account.active && (
        <p className="error-notice">
          This account is inactive. You can still view its records.
        </p>
      )}
      <div className="detail-summary-grid">
        <section
          className="balance-panel detail-balance"
          aria-label="Current account balance"
        >
          <div className="balance-pattern" aria-hidden="true">
            <span />
            <span />
            <span />
          </div>
          <div className="balance-panel-top">
            <span>
              <span className="balance-dot" />
              Current balance
            </span>
            <span className="currency-badge">{account.currency}</span>
          </div>
          <div className="hero-amount" aria-busy={balance.isPending}>
            {balance.isPending ? (
              <span
                className="skeleton skeleton-balance"
                aria-label="Loading balance"
              />
            ) : balance.isError ? (
              <span className="unavailable">Balance unavailable</span>
            ) : balance.data ? (
              formatMoney(balance.data.balance_minor, account.currency)
            ) : (
              "—"
            )}
          </div>
          <div className="balance-panel-bottom">
            <Icon name="shield" width="16" height="16" />
            <span>Your complete account balance.</span>
          </div>
        </section>
        <section className="detail-month-panel" aria-label="Monthly activity">
          <div className="detail-month-heading">
            <div>
              <h2>Monthly movement</h2>
              <p>{monthLabel} · UTC</p>
            </div>
            <label className="month-picker">
              <span className="sr-only">Summary month (UTC)</span>
              <input
                type="month"
                value={ym}
                min="0001-01"
                max="9999-12"
                onChange={(e) => {
                  if (/^\d{4}-\d{2}$/.test(e.target.value))
                    setYm(e.target.value);
                }}
              />
            </label>
          </div>
          <div className="detail-month-stats">
            <Stat
              label="Money in"
              amount={summary.data?.inflow}
              currency={account.currency}
              pending={summary.isPending}
              error={summary.isError}
              icon="down"
            />
            <Stat
              label="Money out"
              amount={summary.data?.outflow}
              currency={account.currency}
              pending={summary.isPending}
              error={summary.isError}
              icon="up"
            />
            <Stat
              label="Net movement"
              amount={summary.data?.net}
              currency={account.currency}
              pending={summary.isPending}
              error={summary.isError}
              icon="transfer"
            />
          </div>
          <p className="monthly-footnote">
            Includes transfers and reversals recorded in this month.
          </p>
        </section>
      </div>
      {(balance.isError || summary.isError) && (
        <div className="error-notice" role="alert">
          <span>Some account figures couldn't be loaded.</span>
          <button
            className="text-button"
            onClick={() => {
              if (balance.isError) void balance.refetch();
              if (summary.isError) void summary.refetch();
            }}
          >
            Try again
          </button>
        </div>
      )}
      <section className="activity-section" aria-labelledby="activity-title">
        <div className="section-heading">
          <div>
            <p className="eyebrow">THE EVERYDAY DETAILS</p>
            <h2 id="activity-title">Activity</h2>
            <p>All time · Newest first</p>
          </div>
          <span className="activity-hint">
            <Icon name="receipt" width="16" height="16" />A story behind every
            entry.
          </span>
        </div>
        {entries.isRefetchError &&
          !entries.isFetchNextPageError &&
          allEntries.length > 0 && (
            <div className="error-notice" role="alert">
              <span>
                Couldn't refresh your activity. Previously loaded entries are
                shown.
              </span>
              <button className="text-button" onClick={() => entries.refetch()}>
                Try again
              </button>
            </div>
          )}
        <div className="activity-table">
          {entries.isPending ? (
            <div
              className="activity-loading"
              aria-busy="true"
              aria-label="Loading transactions"
            >
              {[0, 1, 2].map((i) => (
                <span className="skeleton" key={i} />
              ))}
            </div>
          ) : entries.isError && !allEntries.length ? (
            <div className="activity-empty">
              <Icon name="info" width="30" height="30" />
              <h3>We couldn't load your activity.</h3>
              <p>{apiError(entries.error)}</p>
              <button className="btn" onClick={() => entries.refetch()}>
                Try again
              </button>
            </div>
          ) : !allEntries.length ? (
            <div className="activity-empty">
              <span className="account-symbol tone-sage">
                <Icon name="receipt" width="27" height="27" />
              </span>
              <h3>Your story starts with an entry.</h3>
              <p>
                Record your first income or expense.
                <br />
                We'll keep the details together here.
              </p>
              <button
                className="btn btn-primary"
                onClick={() => open("transaction")}
                disabled={!account.active}
              >
                <Icon name="plus" width="17" height="17" />
                Record first transaction
              </button>
            </div>
          ) : (
            <>
              <div className="activity-table-heading" aria-hidden="true">
                <span>TRANSACTION</span>
                <span>CATEGORY</span>
                <span>DATE</span>
                <span>AMOUNT</span>
              </div>
              <ul className="activity-list">
                {allEntries.map((entry) => (
                  <ActivityRow
                    key={entry.posting_id}
                    entry={entry}
                    category={
                      categories.data?.find(
                        (c) =>
                          c.id ===
                          (entry.category_id ?? entry.suggested_category_id),
                      )?.name
                    }
                    onClick={() => setSelectedId(entry.posting_id)}
                  />
                ))}
              </ul>
              {entries.isFetchNextPageError && (
                <div className="activity-pagination-error" role="alert">
                  Couldn't load more transactions. Your loaded entries are still
                  here.
                </div>
              )}
              <div className="activity-table-footer">
                <span>
                  {allEntries.length}{" "}
                  {allEntries.length === 1 ? "entry" : "entries"} loaded ·
                  Select one to see details
                </span>
                {entries.hasNextPage ? (
                  <button
                    className="text-link"
                    disabled={entries.isFetchingNextPage}
                    onClick={() => entries.fetchNextPage()}
                  >
                    {entries.isFetchingNextPage
                      ? "Loading…"
                      : entries.isFetchNextPageError
                        ? "Retry loading more"
                        : "Load more"}
                    <Icon name="down" width="15" height="15" />
                  </button>
                ) : (
                  <span>You're all caught up.</span>
                )}
              </div>
            </>
          )}
        </div>
      </section>
      <CategoryManager
        categories={categories.data ?? []}
        pending={categories.isPending}
        error={categories.isError}
        onRetry={() => categories.refetch()}
      />
      <p className="accounts-footnote">
        <Icon name="shield" width="15" height="15" />
        Every entry has its equal. Your history stays complete.
      </p>
      {dialog === "transaction" && (
        <TransactionDialog
          account={account}
          onClose={() => setDialog(null)}
          onSaved={onSaved}
        />
      )}
      {dialog === "transfer" && (
        <TransferDialog
          account={account}
          accounts={accountsQuery.data ?? []}
          onClose={() => setDialog(null)}
          onSaved={onSaved}
        />
      )}
      {selected && (
        <TransactionDrawer
          key={selected.posting_id}
          entry={selected}
          account={account}
          categories={categories.data ?? []}
          categoriesPending={categories.isPending}
          categoriesError={categories.isError}
          onRetryCategories={() => categories.refetch()}
          onClose={() => setSelectedId(null)}
          onReversed={onSaved}
          onCategorized={refreshEntries}
          relatedAvailable={allEntries.some(
            (e) => e.transaction_id === relatedId,
          )}
          onRelated={(id) => {
            const related = allEntries.find((e) => e.transaction_id === id);
            if (related) setSelectedId(related.posting_id);
          }}
        />
      )}
    </div>
  );
}

function ActivityRow({
  entry,
  category,
  onClick,
}: {
  entry: Entry;
  category?: string;
  onClick: () => void;
}) {
  const kind = entryKind(entry);
  const positive = entry.amount_minor > 0;
  const title =
    entry.note ||
    (kind === "Transfer"
      ? "Account transfer"
      : kind === "Reversal"
        ? "Transaction reversal"
        : kind === "Income"
          ? "Money received"
          : "Money spent");
  return (
    <li>
      <button
        className="activity-row"
        onClick={onClick}
        aria-label={`View ${title}, ${formatMoney(entry.amount_minor, entry.currency)}`}
      >
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
          </span>
        </span>
        <span
          className={`activity-category ${!entry.category_id && entry.suggested_category_id ? "suggested" : ""}`}
        >
          {!entry.category_id && entry.suggested_category_id && (
            <Icon name="sparkle" width="13" height="13" />
          )}
          {category ?? (entry.category_id ? "Categorized" : "Uncategorized")}
          {!entry.category_id && entry.suggested_category_id && (
            <span className="sr-only"> — suggested, not confirmed</span>
          )}
        </span>
        <time className="activity-date" dateTime={entry.occurred_at}>
          {new Date(entry.occurred_at).toLocaleDateString(undefined, {
            day: "numeric",
            month: "short",
            year: "numeric",
          })}
        </time>
        <span className={`activity-amount ${positive ? "positive" : ""}`}>
          {positive ? "+" : ""}
          {formatMoney(entry.amount_minor, entry.currency)}
          <Icon name="chevron" width="14" height="14" />
        </span>
      </button>
    </li>
  );
}

function Stat({
  label,
  amount,
  currency,
  pending,
  error,
  icon,
}: {
  label: string;
  amount?: number;
  currency: string;
  pending: boolean;
  error: boolean;
  icon: "up" | "down" | "transfer";
}) {
  return (
    <div className="detail-stat">
      <span>
        <Icon name={icon} width="14" height="14" />
        {label}
      </span>
      <strong aria-busy={pending}>
        {pending ? (
          <span
            className="skeleton skeleton-line"
            aria-label={`Loading ${label}`}
          />
        ) : error ? (
          <span className="amount-error">Unavailable</span>
        ) : amount !== undefined ? (
          formatMoney(amount, currency)
        ) : (
          "—"
        )}
      </strong>
    </div>
  );
}

function AccountNotFound() {
  return (
    <div className="activity-empty">
      <Icon name="wallet" width="30" height="30" />
      <h1>Account not found</h1>
      <p>This account isn't available in your workspace.</p>
      <Link to="/accounts" className="btn btn-primary">
        Back to accounts
      </Link>
    </div>
  );
}
