import { useQueries, useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { useState } from "react";
import {
  type Account,
  formatMoney,
  getCurrentBalance,
  getMonthlySummary,
  listAccounts,
} from "../api/endpoints";
import { apiError } from "../lib/errors";
import { readAppearance } from "../lib/account-appearance";
import CreateAccountDialog from "../components/CreateAccountDialog";
import Icon from "../components/Icon";

type Balance = Awaited<ReturnType<typeof getCurrentBalance>>;
type BalanceState = { data?: Balance; isPending: boolean; isError: boolean };

export default function Accounts({ overview = false }: { overview?: boolean }) {
  const accountsQuery = useQuery({
    queryKey: ["accounts"],
    queryFn: listAccounts,
  });
  const accounts =
    accountsQuery.data?.filter((a) => a.kind !== "external") ?? [];
  const balances = useQueries({
    queries: accounts.map((a) => ({
      queryKey: ["balance", a.id],
      queryFn: () => getCurrentBalance(a.id),
    })),
  });
  const [dialogOpen, setDialogOpen] = useState(false);
  const [announcement, setAnnouncement] = useState("");
  const [currencyChoice, setCurrencyChoice] = useState("");
  const [search, setSearch] = useState("");
  const [view, setView] = useState<"grid" | "list">("grid");
  const currencies = [...new Set(accounts.map((a) => a.currency))].sort();
  const currency = currencies.includes(currencyChoice)
    ? currencyChoice
    : (currencies[0] ?? "USD");
  const selected = accounts
    .map((a, index) => ({ ...a, balance: balances[index] }))
    .filter((a) => a.currency === currency);
  const now = new Date();
  const ym = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}`;
  const monthLabel = new Intl.DateTimeFormat(undefined, {
    month: "long",
    year: "numeric",
  }).format(now);
  const summaries = useQueries({
    queries: overview
      ? selected.map((a) => ({
          queryKey: ["summary", a.id, ym],
          queryFn: () => getMonthlySummary(a.id, ym),
        }))
      : [],
  });
  const totalPending =
    accountsQuery.isPending || selected.some((a) => a.balance.isPending);
  const totalError =
    accountsQuery.isError || selected.some((a) => a.balance.isError);
  const total = selected.reduce(
    (sum, a) => sum + (a.balance.data?.balance_minor ?? 0),
    0,
  );
  const summaryPending =
    accountsQuery.isPending || summaries.some((s) => s.isPending);
  const summaryError =
    accountsQuery.isError || summaries.some((s) => s.isError);
  const inflow = summaries.reduce((sum, s) => sum + (s.data?.inflow ?? 0), 0);
  const outflow = summaries.reduce((sum, s) => sum + (s.data?.outflow ?? 0), 0);
  const filtered = accounts.filter((a) =>
    `${a.name} ${a.currency}`
      .toLowerCase()
      .includes(search.toLowerCase().trim()),
  );
  const visible = overview ? accounts.slice(0, 3) : filtered;
  const anyDataError =
    balances.some((b) => b.isError) || summaries.some((s) => s.isError);
  const openCreate = () => {
    setAnnouncement("");
    setDialogOpen(true);
  };

  return (
    <div className="accounts-page">
      <div className="page-heading">
        <div>
          <p className="eyebrow">
            {overview ? "THE BIG PICTURE" : "A PLACE FOR EVERY PURPOSE"}
          </p>
          <h1>
            {overview ? (
              <>
                Your money, <em>in focus.</em>
              </>
            ) : (
              <>Your accounts.</>
            )}
          </h1>
          <p className="page-description">
            {overview
              ? "A little perspective for your everyday finances."
              : "Every balance, beautifully organized. All in one place."}
          </p>
        </div>
        <button
          className="btn btn-primary new-account-button"
          onClick={openCreate}
        >
          <Icon name="plus" width="18" height="18" />
          New account
        </button>
      </div>
      <div
        role="status"
        aria-live="polite"
        className={announcement ? "success-notice" : "sr-only"}
      >
        {announcement && (
          <>
            <Icon name="check" />
            {announcement}
          </>
        )}
      </div>

      {overview && (
        <>
          <div className="overview-toolbar">
            <span className="section-caption">YOUR FINANCIAL SNAPSHOT</span>
            {currencies.length > 1 && (
              <div
                className="currency-switch"
                role="group"
                aria-label="Balance currency"
              >
                {currencies.map((c) => (
                  <button
                    key={c}
                    aria-pressed={currency === c}
                    onClick={() => setCurrencyChoice(c)}
                  >
                    {c}
                  </button>
                ))}
              </div>
            )}
          </div>
          <div className="snapshot-grid">
            <section
              className="balance-panel"
              aria-label={`Total balance in ${currency}`}
            >
              <div className="balance-pattern" aria-hidden="true">
                <span />
                <span />
                <span />
              </div>
              <div className="balance-panel-top">
                <span>
                  <span className="balance-dot" />
                  Total balance
                </span>
                <span className="currency-badge">{currency}</span>
              </div>
              <div className="hero-amount" aria-busy={totalPending}>
                {totalPending ? (
                  <span
                    className="skeleton skeleton-balance"
                    aria-label="Loading balance"
                  />
                ) : totalError ? (
                  <span className="unavailable">Balance unavailable</span>
                ) : (
                  formatMoney(total, currency)
                )}
              </div>
              <p className="balance-caption">
                {accountsQuery.isPending
                  ? "Getting your accounts ready…"
                  : totalError
                    ? "We couldn't load your complete balance."
                    : `Across ${selected.length} ${currency} ${selected.length === 1 ? "account" : "accounts"}`}
              </p>
              <div className="balance-panel-bottom">
                <Icon name="wallet" width="16" height="16" />
                <span>One currency. A clearer picture.</span>
                <Icon name="up" width="17" height="17" />
              </div>
            </section>
            <section
              className="monthly-panel"
              aria-label="Monthly money movement"
            >
              <div className="monthly-heading">
                <h2>This month</h2>
                <span>{monthLabel}</span>
              </div>
              <div className="money-movement">
                <span className="movement-icon income">
                  <Icon name="down" />
                </span>
                <div>
                  <span className="movement-label">Money in</span>
                  <MoneyValue
                    amount={inflow}
                    currency={currency}
                    pending={summaryPending}
                    error={summaryError}
                  />
                </div>
                <span className="movement-currency">{currency}</span>
              </div>
              <div className="money-movement">
                <span className="movement-icon expense">
                  <Icon name="up" />
                </span>
                <div>
                  <span className="movement-label">Money out</span>
                  <MoneyValue
                    amount={outflow}
                    currency={currency}
                    pending={summaryPending}
                    error={summaryError}
                  />
                </div>
                <span className="movement-currency">{currency}</span>
              </div>
              <p className="monthly-footnote">
                Account activity, including transfers.
              </p>
            </section>
          </div>
          {currencies.length > 1 && (
            <p className="currency-note">
              <Icon name="info" width="14" height="14" />
              Balances are grouped by currency. No exchange rates or
              conversions.
            </p>
          )}
        </>
      )}

      {accountsQuery.isError && (
        <div className="error-notice" role="alert">
          <div>
            <Icon name="info" />
            <span>
              {apiError(accountsQuery.error, "We couldn't load your accounts.")}
            </span>
          </div>
          <button
            className="text-button"
            onClick={() => accountsQuery.refetch()}
          >
            Try again
          </button>
        </div>
      )}
      {anyDataError && !accountsQuery.isError && (
        <div className="error-notice" role="alert">
          <div>
            <Icon name="info" />
            <span>Some balances or summaries couldn't be loaded.</span>
          </div>
          <button
            className="text-button"
            onClick={() => {
              balances.forEach((b) => {
                if (b.isError) void b.refetch();
              });
              summaries.forEach((s) => {
                if (s.isError) void s.refetch();
              });
            }}
          >
            Try again
          </button>
        </div>
      )}

      <section className="accounts-section" aria-labelledby="accounts-heading">
        <div className="section-heading">
          <div>
            <h2 id="accounts-heading">
              {overview ? "Your accounts" : "All accounts"}
              {!accountsQuery.isPending && !accountsQuery.isError && (
                <span className="count-badge">{accounts.length}</span>
              )}
            </h2>
            {overview && <p>Different purposes. One organized place.</p>}
          </div>
          {overview ? (
            <Link to="/accounts" className="text-link">
              View all accounts
              <Icon name="arrow" width="16" height="16" />
            </Link>
          ) : (
            <div
              className="view-switch"
              role="group"
              aria-label="Account layout"
            >
              <button
                aria-label="Grid view"
                aria-pressed={view === "grid"}
                onClick={() => setView("grid")}
              >
                <Icon name="grid" width="16" height="16" />
              </button>
              <button
                aria-label="List view"
                aria-pressed={view === "list"}
                onClick={() => setView("list")}
              >
                <Icon name="list" width="18" height="18" />
              </button>
            </div>
          )}
        </div>
        {!overview && accounts.length > 0 && (
          <div className="account-search">
            <Icon name="search" width="18" height="18" />
            <input
              aria-label="Search accounts"
              placeholder="Find an account by name or currency…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
            {search && (
              <button
                className="icon-button"
                aria-label="Clear search"
                onClick={() => setSearch("")}
              >
                <Icon name="close" width="16" height="16" />
              </button>
            )}
          </div>
        )}
        {accountsQuery.isPending && (
          <div
            className="account-grid"
            aria-label="Loading accounts"
            aria-busy="true"
          >
            {[0, 1, 2].map((key) => (
              <div className="account-card card-loading" key={key}>
                <span className="skeleton skeleton-icon" />
                <span className="skeleton skeleton-line" />
                <span className="skeleton skeleton-line short" />
              </div>
            ))}
          </div>
        )}
        {!accountsQuery.isPending &&
          !accountsQuery.isError &&
          accounts.length === 0 && (
            <div className="empty-accounts">
              <div className="empty-art" aria-hidden="true">
                <span className="empty-card back">
                  <Icon name="bank" width="30" height="30" />
                </span>
                <span className="empty-card front">
                  <Icon name="wallet" width="30" height="30" />
                  <span className="empty-card-lines" />
                </span>
                <Icon
                  name="sparkle"
                  className="empty-spark"
                  width="31"
                  height="31"
                />
              </div>
              <h3>Good things start with a first account.</h3>
              <p>
                Everyday spending, a rainy day fund, or something bigger.
                <br />
                Give your money a place to belong.
              </p>
              <button className="btn btn-primary" onClick={openCreate}>
                <Icon name="plus" width="17" height="17" />
                Create your first account
              </button>
            </div>
          )}
        {visible.length > 0 && (
          <div
            className={`account-grid ${!overview && view === "list" ? "account-list" : ""}`}
          >
            {visible.map((account) => (
              <AccountCard
                key={account.id}
                account={account}
                balance={
                  balances[accounts.findIndex((a) => a.id === account.id)]
                }
              />
            ))}
            {overview && accounts.length > 0 && accounts.length < 3 && (
              <button className="add-account-card" onClick={openCreate}>
                <span>
                  <Icon name="plus" width="22" height="22" />
                </span>
                <strong>Room for your next goal.</strong>
                <span>
                  Create another account
                  <Icon name="arrow" width="15" height="15" />
                </span>
              </button>
            )}
          </div>
        )}
        {!accountsQuery.isError &&
          accounts.length > 0 &&
          visible.length === 0 && (
            <div className="search-empty">
              <Icon name="search" width="28" height="28" />
              <h3>No accounts found</h3>
              <p>Try another name or currency.</p>
              <button className="btn" onClick={() => setSearch("")}>
                Clear search
              </button>
            </div>
          )}
      </section>

      {overview && (
        <section className="next-step-panel">
          <div className="ledger-art" aria-hidden="true">
            <span className="ledger-art-circle">
              <Icon name="shield" width="30" height="30" />
            </span>
            <span className="ledger-rule" />
            <span className="ledger-rule" />
          </div>
          <div>
            <p className="eyebrow">A LITTLE STRUCTURE. A LOT OF CLARITY.</p>
            <h2>
              Every detail. <em>In balance.</em>
            </h2>
            <p>
              {accounts.length
                ? "Your next entry is a step toward a clearer picture."
                : "Start with an account. Add an entry. Let the picture come together."}
            </p>
          </div>
          {accounts.length ? (
            <Link className="btn" to={`/accounts/${accounts[0].id}`}>
              Record a transaction
              <Icon name="arrow" width="17" height="17" />
            </Link>
          ) : (
            <button className="btn" onClick={openCreate}>
              Let's get started
              <Icon name="arrow" width="17" height="17" />
            </button>
          )}
        </section>
      )}
      {!overview && (
        <p className="accounts-footnote">
          <Icon name="shield" width="15" height="15" />
          Every transaction is recorded with a balancing entry.
        </p>
      )}
      {dialogOpen && (
        <CreateAccountDialog
          onClose={() => setDialogOpen(false)}
          onCreated={(name) =>
            setAnnouncement(`${name} is ready. Your next chapter starts here.`)
          }
        />
      )}
    </div>
  );
}

function MoneyValue({
  amount,
  currency,
  pending,
  error,
}: {
  amount: number;
  currency: string;
  pending: boolean;
  error: boolean;
}) {
  return (
    <strong className="movement-value" aria-busy={pending}>
      {pending ? (
        <span className="skeleton skeleton-line" aria-label="Loading amount" />
      ) : error ? (
        <span className="amount-error">Unavailable</span>
      ) : (
        formatMoney(amount, currency)
      )}
    </strong>
  );
}

function AccountCard({
  account,
  balance,
}: {
  account: Account;
  balance: BalanceState;
}) {
  const appearance = readAppearance(account.id);
  return (
    <Link
      to={`/accounts/${account.id}`}
      className={`account-card accent-${appearance.color}`}
    >
      <div className="account-card-top">
        <span className={`account-symbol tone-${appearance.color}`}>
          <Icon name={appearance.icon} width="23" height="23" />
        </span>
        <span className="account-currency">{account.currency}</span>
      </div>
      <div className="account-card-name">
        <h3>{account.name}</h3>
        <span className="account-status">
          <span className={account.active ? "active-dot" : "inactive-dot"} />
          {account.active ? "Active account" : "Inactive account"}
        </span>
      </div>
      <div className="account-card-bottom">
        <div>
          <span className="balance-label">Current balance</span>
          <strong aria-busy={balance.isPending}>
            {balance.isPending ? (
              <span
                className="skeleton skeleton-line"
                aria-label="Loading balance"
              />
            ) : balance.isError ? (
              <span className="amount-error">Unavailable</span>
            ) : balance.data ? (
              formatMoney(balance.data.balance_minor, account.currency)
            ) : (
              "—"
            )}
          </strong>
        </div>
        <span className="account-open">
          <Icon name="up" width="19" height="19" />
          <span className="sr-only">Open {account.name}</span>
        </span>
      </div>
    </Link>
  );
}
