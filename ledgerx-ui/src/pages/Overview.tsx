import { useState } from "react";
import { Link } from "react-router-dom";
import { useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  formatMoney,
  getCurrentBalance,
  getSpending,
  listAccounts,
} from "../api/endpoints";
import { apiError } from "../lib/errors";
import {
  reportLabel,
  reportRange,
  type ReportSelection,
} from "../lib/report-period";
import { defaultReportSelection } from "../lib/spending";
import { refreshLedger } from "../lib/refresh-ledger";
import { AccountCard } from "./Accounts";
import CreateAccountDialog from "../components/CreateAccountDialog";
import TransactionDialog from "../components/TransactionDialog";
import PeriodPicker from "../components/PeriodPicker";
import Icon from "../components/Icon";
import SuccessNotice from "../components/SuccessNotice";
import FitMoney from "../components/FitMoney";
import { chooseEntryAccount } from "../lib/transactions";

export default function Overview() {
  const qc = useQueryClient();
  const [period, setPeriod] = useState<ReportSelection>(() =>
    defaultReportSelection("monthly"),
  );
  const [currencyChoice, setCurrencyChoice] = useState("");
  const [dialog, setDialog] = useState<"account" | "transaction" | null>(null);
  const [message, setMessage] = useState("");
  const [showAll, setShowAll] = useState(false);
  const accountsQuery = useQuery({
    queryKey: ["accounts"],
    queryFn: listAccounts,
  });
  const accounts = accountsQuery.data?.filter((a) => a.kind === "normal") ?? [];
  const active = accounts.filter((a) => a.active);
  const currencies = [...new Set(accounts.map((a) => a.currency))].sort();
  const primaryCurrency =
    [...accounts].sort((a, b) => a.id - b.id)[0]?.currency ?? "USD";
  const currency = currencies.includes(currencyChoice)
    ? currencyChoice
    : primaryCurrency;
  const entryAccount = chooseEntryAccount(active, currency);
  const selected = accounts.filter((a) => a.currency === currency);
  const balances = useQueries({
    queries: selected.map((a) => ({
      queryKey: ["balance", a.id],
      queryFn: () => getCurrentBalance(a.id),
    })),
  });
  const range = reportRange(period);
  const spending = useQuery({
    queryKey: ["spending", period, currency],
    queryFn: () => getSpending(period, currency),
    enabled: !!range && accounts.length > 0,
  });
  const balancePending =
    accountsQuery.isPending || balances.some((b) => b.isPending);
  const balanceError = accountsQuery.isError || balances.some((b) => b.isError);
  const total = balances.reduce(
    (sum, b) => sum + BigInt(b.data?.balance_minor ?? 0),
    0n,
  );
  const totalSupported =
    total >= -BigInt(Number.MAX_SAFE_INTEGER) &&
    total <= BigInt(Number.MAX_SAFE_INTEGER);
  const categories = spending.data?.categories ?? [];
  const maxCategory = Math.max(
    1,
    ...categories.map((c) => Math.abs(c.spending_minor)),
  );
  const uncategorized = categories.find((c) => c.category_id === 0);
  const saved = (notice: string) => {
    setDialog(null);
    setMessage(notice);
    void refreshLedger(qc);
  };
  const activityParams = new URLSearchParams({
    currency,
    ...(range?.start && range.end
      ? { start_date: range.start, end_date: range.end }
      : {}),
  });

  return (
    <div className="spending-page">
      <div className="page-heading">
        <div>
          <h1>Overview</h1>
        </div>
        <button
          className="btn btn-primary"
          disabled={accountsQuery.isPending || accountsQuery.isError}
          onClick={() => {
            setMessage("");
            setDialog(active.length ? "transaction" : "account");
          }}
        >
          <Icon name="plus" width="18" height="18" />
          {active.length
            ? "Add transaction"
            : accounts.length
              ? "New account"
              : "Create your first account"}
        </button>
      </div>
      <SuccessNotice message={message} onDismiss={() => setMessage("")} />
      {!!accounts.length && !active.length && (
        <p className="muted">
          All accounts are archived.{" "}
          <Link className="text-link" to="/accounts">
            Restore an account
          </Link>{" "}
          or create a new one to add transactions.
        </p>
      )}
      {accountsQuery.isError && (
        <div role="alert" className="error-notice">
          <span>{apiError(accountsQuery.error)}</span>
          <button
            className="text-button"
            onClick={() => accountsQuery.refetch()}
          >
            Try again
          </button>
        </div>
      )}
      {accountsQuery.isPending ? (
        <p role="status" className="spending-empty">
          Loading accounts…
        </p>
      ) : accountsQuery.isError ? null : !accounts.length ? (
        <section className="empty-accounts">
          <span className="account-symbol tone-sage">
            <Icon name="wallet" width="28" height="28" />
          </span>
          <h2>No accounts yet</h2>
          <p>
            Create an account with your starting balance, then record purchases
            as you go. No bank connection required.
          </p>
          <button
            className="btn btn-primary"
            onClick={() => setDialog("account")}
          >
            Create an account
            <Icon name="arrow" />
          </button>
        </section>
      ) : (
        <>
          <div className="spending-stat-grid">
            <section className="spending-stat current-balance">
              <span>Current balance</span>
              <strong aria-busy={balancePending}>
                {balancePending ? (
                  "Loading…"
                ) : balanceError ? (
                  "Unavailable"
                ) : !totalSupported ? (
                  "Out of range"
                ) : (
                  <FitMoney minor={Number(total)} currency={currency} />
                )}
              </strong>
              <p>
                Across {selected.length} {currency}{" "}
                {selected.length === 1 ? "account" : "accounts"} · current, all
                dates
                {selected.some((a) => !a.active)
                  ? " · includes archived accounts"
                  : ""}
              </p>
            </section>
            <section className="spending-stat">
              <span>
                <Icon name="down" width="17" height="17" />
                Income
              </span>
              <strong aria-busy={spending.isFetching}>
                {!range ? (
                  "—"
                ) : spending.isError ? (
                  "Unavailable"
                ) : spending.isPending ? (
                  "Loading…"
                ) : (
                  <FitMoney
                    minor={spending.data?.income_minor ?? 0}
                    currency={currency}
                  />
                )}
              </strong>
              <p>{reportLabel(period)}</p>
            </section>
            <section className="spending-stat">
              <span>
                <Icon name="up" width="17" height="17" />
                Net spending
              </span>
              <strong aria-busy={spending.isFetching}>
                {!range ? (
                  "—"
                ) : spending.isError ? (
                  "Unavailable"
                ) : spending.isPending ? (
                  "Loading…"
                ) : (
                  <FitMoney
                    minor={spending.data?.spending_minor ?? 0}
                    currency={currency}
                  />
                )}
              </strong>
              <p>Purchases minus refunds · {reportLabel(period)}</p>
            </section>
          </div>
          <div className="spending-controls">
            <PeriodPicker
              value={period}
              onChange={setPeriod}
              showTimezone={false}
            />
            {currencies.length > 1 && (
              <div className="spending-currency">
                <label htmlFor="spending-currency" className="label">
                  Currency
                </label>
                <select
                  id="spending-currency"
                  className="input"
                  value={currency}
                  onChange={(e) => setCurrencyChoice(e.target.value)}
                >
                  {(currencies.length ? currencies : [currency]).map((code) => (
                    <option key={code}>{code}</option>
                  ))}
                </select>
              </div>
            )}
          </div>
          {!range && (
            <p role="alert" className="form-error">
              Choose a valid period. The end date must be on or after the start
              date.
            </p>
          )}
          {!balancePending && !balanceError && !totalSupported && (
            <p role="alert" className="form-error">
              The combined balance exceeds the supported exact range. View
              individual account balances below.
            </p>
          )}
          {balanceError && !accountsQuery.isError && (
            <div role="alert" className="error-notice">
              <span>Couldn't load the complete balance.</span>
              <button
                className="text-button"
                onClick={() =>
                  balances.forEach((b) => {
                    if (b.isError) void b.refetch();
                  })
                }
              >
                Try again
              </button>
            </div>
          )}
          {spending.isError && (
            <div role="alert" className="error-notice">
              <span>{apiError(spending.error, "Couldn't load spending.")}</span>
              <button
                className="text-button"
                onClick={() => spending.refetch()}
              >
                Try again
              </button>
            </div>
          )}
          <div className="spending-review-grid">
            <section
              className="category-spending-panel"
              aria-labelledby="category-spending-title"
            >
              <div className="section-heading">
                <div>
                  <h2 id="category-spending-title">Spending by category</h2>
                </div>
                <Link className="text-link" to={`/activity?${activityParams}`}>
                  View activity
                  <Icon name="arrow" width="16" height="16" />
                </Link>
              </div>
              {!range ? (
                <p className="muted">Select a valid period above.</p>
              ) : spending.isPending ? (
                <p role="status">Loading your spending…</p>
              ) : spending.isError ? (
                <p className="muted">Your category breakdown is unavailable.</p>
              ) : !categories.length ? (
                <div className="spending-empty">
                  <Icon name="receipt" width="28" height="28" />
                  <h3>No spending recorded for this period.</h3>
                  <p>Add a purchase, or choose another period.</p>
                </div>
              ) : (
                <ul className="spending-category-list">
                  {(showAll ? categories : categories.slice(0, 5)).map((c) => (
                    <li key={c.category_id}>
                      <div>
                        <span>{c.name}</span>
                        <strong>
                          {formatMoney(c.spending_minor, currency)}
                        </strong>
                      </div>
                      <div
                        className={`category-bar ${c.spending_minor < 0 ? "net-refund" : ""}`}
                        aria-hidden="true"
                      >
                        <span
                          style={{
                            width: `${(Math.abs(c.spending_minor) / maxCategory) * 100}%`,
                          }}
                        />
                      </div>
                      <small>
                        {c.spending_minor < 0
                          ? "More refunded than spent"
                          : `${c.count} ${c.count === 1 ? "record" : "records"}`}
                      </small>
                    </li>
                  ))}
                </ul>
              )}
              {categories.length > 5 && (
                <button
                  className="text-button"
                  onClick={() => setShowAll(!showAll)}
                >
                  {showAll
                    ? "Show fewer categories"
                    : `Show all ${categories.length} categories`}
                </button>
              )}
            </section>
            {!!uncategorized && (
              <aside className="review-prompt">
                <p>
                  {uncategorized.count} spending{" "}
                  {uncategorized.count === 1 ? "record needs" : "records need"}{" "}
                  a category.
                </p>
                <Link
                  className="text-link"
                  to={`/activity?${activityParams}&category_id=0`}
                >
                  Review uncategorized
                  <Icon name="arrow" width="16" height="16" />
                </Link>
              </aside>
            )}
          </div>
          <details className="spending-explanation">
            <summary>How these numbers work</summary>
            <p>
              Income and spending exclude transfers between your accounts and
              starting balances. Refunds reduce spending in the period received.
              Reversed transactions and their reversals are excluded, updating
              the original period. Balances and CSV records keep every ledger
              entry.
            </p>
            <p>
              Periods use UTC calendar dates and include the end date.
              Currencies stay separate; there are no exchange-rate conversions.
              These are recorded amounts, not a forecast of what is safe to
              spend.
            </p>
          </details>
          <section className="accounts-section">
            <div className="section-heading">
              <div>
                <h2>
                  Your accounts{" "}
                  <span className="count-badge">{selected.length}</span>
                </h2>
              </div>
              <div className="heading-actions">
                <Link to="/accounts" className="text-link">
                  View all
                  <Icon name="arrow" width="16" height="16" />
                </Link>
                <button className="btn" onClick={() => setDialog("account")}>
                  <Icon name="plus" width="16" height="16" />
                  New account
                </button>
              </div>
            </div>
            <div className="account-grid">
              {selected.slice(0, 3).map((a, i) => (
                <AccountCard key={a.id} account={a} balance={balances[i]} />
              ))}
            </div>
          </section>
        </>
      )}
      {dialog === "account" && (
        <CreateAccountDialog
          onClose={() => setDialog(null)}
          onCreated={(name) => saved(`${name} is ready.`)}
        />
      )}
      {dialog === "transaction" && entryAccount && (
        <TransactionDialog
          account={entryAccount}
          accounts={active}
          onClose={() => setDialog(null)}
          onSaved={saved}
        />
      )}
    </div>
  );
}
