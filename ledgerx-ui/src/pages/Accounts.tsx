import { useQueries, useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Link } from "react-router-dom";
import {
  type Account,
  formatMoney,
  getCurrentBalance,
  listAccounts,
} from "../api/endpoints";
import { apiError } from "../lib/errors";
import { readAppearance } from "../lib/account-appearance";
import CreateAccountDialog from "../components/CreateAccountDialog";
import Icon from "../components/Icon";

type Balance = Awaited<ReturnType<typeof getCurrentBalance>>;
type BalanceState = { data?: Balance; isPending: boolean; isError: boolean };

export default function Accounts() {
  const accountsQuery = useQuery({
    queryKey: ["accounts"],
    queryFn: listAccounts,
  });
  const accounts = accountsQuery.data?.filter((a) => a.kind === "normal") ?? [];
  const balances = useQueries({
    queries: accounts.map((a) => ({
      queryKey: ["balance", a.id],
      queryFn: () => getCurrentBalance(a.id),
    })),
  });
  const [dialogOpen, setDialogOpen] = useState(false);
  const [announcement, setAnnouncement] = useState("");
  const [search, setSearch] = useState("");
  const [view, setView] = useState<"grid" | "list">("grid");
  const filtered = accounts.filter((a) =>
    `${a.name} ${a.currency}`
      .toLowerCase()
      .includes(search.toLowerCase().trim()),
  );
  const openCreate = () => {
    setAnnouncement("");
    setDialogOpen(true);
  };
  return (
    <div className="accounts-page">
      <div className="page-heading">
        <div>
          <p className="eyebrow">A PLACE FOR EVERY PURPOSE</p>
          <h1>Your accounts.</h1>
          <p className="page-description">
            Cash, everyday spending, and savings. Keep them together.
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
      {announcement && (
        <p role="status" className="success-notice">
          <Icon name="check" />
          {announcement}
        </p>
      )}
      {accountsQuery.isError && (
        <div role="alert" className="error-notice">
          <span>
            {apiError(accountsQuery.error, "Couldn't load your accounts.")}
          </span>
          <button
            className="text-button"
            onClick={() => accountsQuery.refetch()}
          >
            Try again
          </button>
        </div>
      )}
      {balances.some((b) => b.isError) && (
        <div role="alert" className="error-notice">
          <span>Some balances couldn't be loaded.</span>
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
      <section className="accounts-section" aria-labelledby="accounts-heading">
        <div className="section-heading">
          <h2 id="accounts-heading">
            All accounts{" "}
            {!accountsQuery.isPending && !accountsQuery.isError && (
              <span className="count-badge">{accounts.length}</span>
            )}
          </h2>
          {accounts.length > 0 && (
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
        {accounts.length > 0 && (
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
          !accounts.length && (
            <div className="empty-accounts">
              <span className="account-symbol tone-sage">
                <Icon name="wallet" width="28" height="28" />
              </span>
              <h3>Start with your first account.</h3>
              <p>
                Give it a name and starting balance, then add your everyday
                transactions.
              </p>
              <button className="btn btn-primary" onClick={openCreate}>
                <Icon name="plus" width="17" height="17" />
                Create your first account
              </button>
            </div>
          )}
        {filtered.length > 0 && (
          <div
            className={`account-grid ${view === "list" ? "account-list" : ""}`}
          >
            {filtered.map((a) => (
              <AccountCard
                key={a.id}
                account={a}
                balance={
                  balances[accounts.findIndex((item) => item.id === a.id)]
                }
              />
            ))}
          </div>
        )}
        {accounts.length > 0 && !filtered.length && (
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
      <p className="accounts-footnote">
        <Icon name="info" width="15" height="15" />
        Balances include starting money and all recorded movements.
      </p>
      {dialogOpen && (
        <CreateAccountDialog
          onClose={() => setDialogOpen(false)}
          onCreated={(name) => setAnnouncement(`${name} is ready.`)}
        />
      )}
    </div>
  );
}

export function AccountCard({
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
