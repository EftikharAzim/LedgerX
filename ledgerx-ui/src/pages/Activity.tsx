import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  listActivity,
  listAccounts,
  listCategories,
  type ActivityEntry,
  type ActivityFilters,
} from "../api/endpoints";
import { apiError } from "../lib/errors";
import { reportRange } from "../lib/report-period";
import { refreshLedger } from "../lib/refresh-ledger";
import ActivityRow from "../components/ActivityRow";
import TransactionDrawer from "../components/TransactionDrawer";
import TransactionDialog from "../components/TransactionDialog";
import Icon from "../components/Icon";
import SuccessNotice from "../components/SuccessNotice";
import { chooseEntryAccount } from "../lib/transactions";

const filterNames = [
  "transaction_id",
  "import_id",
  "search",
  "account_id",
  "category_id",
  "type",
  "currency",
  "start_date",
  "end_date",
] as const;

export default function Activity() {
  const qc = useQueryClient();
  const [params, setParams] = useSearchParams();
  const filters = Object.fromEntries(
    filterNames
      .filter((name) => params.get(name))
      .map((name) => [name, params.get(name)!]),
  ) as ActivityFilters;
  const activeFilterCount = Object.keys(filters).filter(
    (name) => name !== "search",
  ).length;
  const [filtersOpen, setFiltersOpen] = useState(activeFilterCount > 0);
  const [draftSearch, setDraftSearch] = useState(filters.search ?? "");
  const [selected, setSelected] = useState<ActivityEntry | null>(null);
  const [adding, setAdding] = useState(false);
  const [message, setMessage] = useState("");
  const accountsQuery = useQuery({
    queryKey: ["accounts"],
    queryFn: listAccounts,
  });
  const accounts = accountsQuery.data?.filter((a) => a.kind === "normal") ?? [];
  const active = accounts.filter((a) => a.active);
  const entryAccount = chooseEntryAccount(active, filters.currency);
  const categories = useQuery({
    queryKey: ["categories"],
    queryFn: listCategories,
  });
  const datesValid =
    (!filters.start_date && !filters.end_date) ||
    !!reportRange({
      type: "custom",
      start_date: filters.start_date ?? "",
      end_date: filters.end_date ?? "",
    });
  const entries = useInfiniteQuery({
    queryKey: ["activity", filters],
    queryFn: ({ pageParam }) => listActivity(filters, pageParam || undefined),
    initialPageParam: "",
    getNextPageParam: (last) => last.next_cursor || undefined,
    enabled: datesValid && accountsQuery.isSuccess,
  });
  const allEntries = entries.data?.pages.flatMap((p) => p.entries) ?? [];
  const selectedAccount = accounts.find((a) => a.id === selected?.account_id);
  const relatedId = selected?.reversal_of ?? selected?.reversed_by;
  const updateFilter = (name: (typeof filterNames)[number], value: string) => {
    setMessage("");
    setParams(
      (previous) => {
        const next = new URLSearchParams(previous);
        if (value) next.set(name, value);
        else next.delete(name);
        return next;
      },
      { replace: true, preventScrollReset: true, flushSync: true },
    );
  };
  const saved = (notice: string) => {
    setAdding(false);
    setSelected(null);
    setMessage(notice);
    void refreshLedger(qc);
  };

  return (
    <div className="activity-page">
      <div className="page-heading">
        <div>
          <h1>Activity</h1>
        </div>
        {active.length > 0 ? (
          <button
            className="btn btn-primary"
            onClick={() => {
              setMessage("");
              setAdding(true);
            }}
          >
            <Icon name="plus" width="18" height="18" />
            Add transaction
          </button>
        ) : (
          <Link className="btn" to="/accounts">
            Manage accounts
            <Icon name="arrow" width="17" height="17" />
          </Link>
        )}
      </div>
      <SuccessNotice message={message} onDismiss={() => setMessage("")} />
      {filters.transaction_id && (
        <p className="search-applied">
          Showing transaction #{filters.transaction_id}.{" "}
          <button
            className="text-button"
            onClick={() => updateFilter("transaction_id", "")}
          >
            Show all matching activity
          </button>
        </p>
      )}
      {filters.import_id && (
        <p className="search-applied">
          Showing records from import #{filters.import_id}.{" "}
          <Link
            className="text-link"
            to={`/imports?import_id=${filters.import_id}`}
          >
            View import receipt
          </Link>
        </p>
      )}
      <section className="activity-filters" aria-label="Find transactions">
        <form
          className="activity-search"
          onSubmit={(e) => {
            e.preventDefault();
            updateFilter("search", draftSearch.trim());
          }}
        >
          <label className="sr-only" htmlFor="activity-search">
            Search transactions
          </label>
          <div className="account-search">
            <Icon name="search" width="18" height="18" />
            <input
              id="activity-search"
              placeholder="Search notes, accounts, categories or TX reference…"
              value={draftSearch}
              maxLength={120}
              onChange={(e) => setDraftSearch(e.target.value)}
            />
          </div>
          <button className="btn" type="submit">
            Search
          </button>
          <button
            type="button"
            className="btn"
            aria-expanded={filtersOpen}
            aria-controls="activity-filter-controls"
            onClick={() => setFiltersOpen(!filtersOpen)}
          >
            Filters{activeFilterCount > 0 ? ` (${activeFilterCount})` : ""}
          </button>
        </form>
        {filters.search && (
          <p className="search-applied">
            Results for “{filters.search}”{" "}
            <button
              className="text-button"
              onClick={() => {
                setDraftSearch("");
                updateFilter("search", "");
              }}
            >
              Clear search
            </button>
          </p>
        )}
        <div id="activity-filter-controls" hidden={!filtersOpen}>
          <div className="activity-filter-grid">
            <div>
              <label className="label" htmlFor="activity-account">
                Account
              </label>
              <select
                id="activity-account"
                className="input"
                value={filters.account_id ?? ""}
                disabled={accountsQuery.isPending || accountsQuery.isError}
                onChange={(e) => updateFilter("account_id", e.target.value)}
              >
                <option value="">All accounts</option>
                {accounts.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name} · {a.currency}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="label" htmlFor="activity-category">
                Category
              </label>
              <select
                id="activity-category"
                className="input"
                value={filters.category_id ?? ""}
                disabled={categories.isPending || categories.isError}
                onChange={(e) => updateFilter("category_id", e.target.value)}
              >
                <option value="">All categories</option>
                <option value="0">Uncategorized</option>
                {categories.data?.map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="label" htmlFor="activity-type">
                Type
              </label>
              <select
                id="activity-type"
                className="input"
                value={filters.type ?? ""}
                onChange={(e) => updateFilter("type", e.target.value)}
              >
                <option value="">All types</option>
                <option value="expense">Expenses</option>
                <option value="income">Income</option>
                <option value="refund">Refunds</option>
                <option value="transfer">Transfers</option>
                <option value="opening_balance">Starting balances</option>
                <option value="reversal">Reversals</option>
              </select>
            </div>
          </div>
          <details
            className="activity-date-filters"
            open={
              !!(filters.start_date || filters.end_date || filters.currency)
            }
          >
            <summary>Dates and currency</summary>
            <div className="activity-filter-grid">
              <div>
                <label className="label" htmlFor="activity-start">
                  Start date · UTC
                </label>
                <input
                  id="activity-start"
                  className="input"
                  type="date"
                  value={filters.start_date ?? ""}
                  onChange={(e) => updateFilter("start_date", e.target.value)}
                />
              </div>
              <div>
                <label className="label" htmlFor="activity-end">
                  End date · UTC, included
                </label>
                <input
                  id="activity-end"
                  className="input"
                  type="date"
                  min={filters.start_date}
                  value={filters.end_date ?? ""}
                  onChange={(e) => updateFilter("end_date", e.target.value)}
                />
              </div>
              <div>
                <label className="label" htmlFor="activity-currency">
                  Currency
                </label>
                <select
                  id="activity-currency"
                  className="input"
                  value={filters.currency ?? ""}
                  onChange={(e) => updateFilter("currency", e.target.value)}
                >
                  <option value="">All currencies</option>
                  {[...new Set(accounts.map((a) => a.currency))]
                    .sort()
                    .map((code) => (
                      <option key={code}>{code}</option>
                    ))}
                </select>
              </div>
            </div>
          </details>
        </div>
        {!datesValid && (
          <p role="alert" className="field-error">
            Choose both dates, with the end on or after the start.
          </p>
        )}
        {Object.keys(filters).length > 0 && (
          <button
            className="text-button"
            onClick={() => {
              setDraftSearch("");
              setFiltersOpen(false);
              setMessage("");
              setParams(
                {},
                { replace: true, preventScrollReset: true, flushSync: true },
              );
            }}
          >
            Clear all filters
          </button>
        )}
      </section>
      {(accountsQuery.isError || categories.isError) && (
        <div role="alert" className="error-notice">
          <span>Couldn't load some account or category details.</span>
          <button
            className="text-button"
            onClick={() => {
              if (accountsQuery.isError) void accountsQuery.refetch();
              if (categories.isError) void categories.refetch();
            }}
          >
            Try again
          </button>
        </div>
      )}
      <section
        className="activity-table workspace-activity"
        aria-labelledby="activity-title"
      >
        <div className="section-heading">
          <div>
            <h2 id="activity-title">
              {Object.keys(filters).length
                ? "Matching activity"
                : "All activity"}
            </h2>
            <p>Transfers appear once. Open a record for details.</p>
          </div>
          <Link className="text-link" to="/categories">
            Manage categories
            <Icon name="arrow" width="15" height="15" />
          </Link>
        </div>
        {!datesValid ? (
          <p className="spending-empty">
            Choose a valid date range to see matching records.
          </p>
        ) : entries.isError ? (
          <div role="alert" className="error-notice">
            <span>
              {apiError(entries.error, "Couldn't load your activity.")}
            </span>
            <button className="text-button" onClick={() => entries.refetch()}>
              Try again
            </button>
          </div>
        ) : entries.isPending ? (
          <p role="status" className="spending-empty">
            Loading your activity…
          </p>
        ) : allEntries.length === 0 ? (
          <div className="spending-empty">
            <Icon name="search" width="28" height="28" />
            <h3>No matching transactions.</h3>
            <p>
              {Object.keys(filters).length
                ? "Try changing or clearing your filters."
                : "Record your first purchase to start building a clearer picture."}
            </p>
          </div>
        ) : (
          <>
            <div className="activity-table-heading" aria-hidden="true">
              <span>Transaction & account</span>
              <span>Category</span>
              <span>Date</span>
              <span>Amount</span>
            </div>
            <ul className="activity-entry-list">
              {allEntries.map((entry) => (
                <ActivityRow
                  key={entry.transaction_id}
                  entry={entry}
                  accountLabel={`${entry.account_name}${entry.counterparty_account_name ? ` → ${entry.counterparty_account_name}` : ""}`}
                  category={
                    categories.data?.find(
                      (c) =>
                        c.id ===
                        (entry.category_id ?? entry.suggested_category_id),
                    )?.name
                  }
                  onClick={() => setSelected(entry)}
                />
              ))}
            </ul>
            <div className="activity-table-footer">
              <span>
                {allEntries.length}{" "}
                {allEntries.length === 1 ? "record" : "records"} shown · times
                displayed locally; imported date-only records keep the source
                date
              </span>
              {entries.hasNextPage && (
                <button
                  className="btn"
                  disabled={entries.isFetchingNextPage}
                  onClick={() => entries.fetchNextPage()}
                >
                  {entries.isFetchingNextPage ? "Loading…" : "Load more"}
                </button>
              )}
            </div>
          </>
        )}
      </section>
      {adding && entryAccount && (
        <TransactionDialog
          account={entryAccount}
          accounts={active}
          onClose={() => setAdding(false)}
          onSaved={saved}
        />
      )}
      {selected && selectedAccount && (
        <TransactionDrawer
          key={selected.transaction_id}
          entry={selected}
          account={selectedAccount}
          counterpartyName={selected.counterparty_account_name}
          categories={categories.data ?? []}
          categoriesPending={categories.isPending}
          categoriesError={categories.isError}
          onRetryCategories={() => {
            void categories.refetch();
          }}
          onClose={() => setSelected(null)}
          onReversed={saved}
          onCategorized={() => {
            setSelected(null);
            setMessage("Category updated.");
            return refreshLedger(qc);
          }}
          relatedAvailable={allEntries.some(
            (e) => e.transaction_id === relatedId,
          )}
          onRelated={(id) => {
            const related = allEntries.find((e) => e.transaction_id === id);
            if (related) setSelected(related);
          }}
        />
      )}
    </div>
  );
}
