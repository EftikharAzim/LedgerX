import { useState } from "react";
import { Link } from "react-router-dom";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import {
  listAccounts,
  listCategories,
  listEntries,
  formatMoney,
} from "../api/endpoints";
import { apiError } from "../lib/errors";
import CreateCategoryDialog from "../components/CreateCategoryDialog";
import SuggestionDialog from "../components/SuggestionDialog";
import Icon from "../components/Icon";

const tones = ["sage", "apricot", "lavender", "sky"];

export default function Categories() {
  const categories = useQuery({
    queryKey: ["categories"],
    queryFn: listCategories,
  });
  const accountsQuery = useQuery({
    queryKey: ["accounts"],
    queryFn: listAccounts,
  });
  const accounts = accountsQuery.data?.filter((a) => a.kind === "normal") ?? [];
  const [search, setSearch] = useState("");
  const [choice, setChoice] = useState("");
  const [creating, setCreating] = useState(false);
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [notice, setNotice] = useState("");
  const account = accounts.find((a) => a.id === Number(choice)) ?? accounts[0];
  const entries = useInfiniteQuery({
    queryKey: ["entries", account?.id],
    queryFn: ({ pageParam }) =>
      listEntries(account!.id, pageParam || undefined),
    initialPageParam: "",
    getNextPageParam: (last) => last.next_cursor || undefined,
    enabled: !!account,
    refetchInterval: 10000,
  });
  const loaded = entries.data?.pages.flatMap((page) => page.entries) ?? [];
  const suggestions = loaded.filter(
    (e) => !e.category_id && !!e.suggested_category_id,
  );
  const selected = suggestions.find((e) => e.posting_id === selectedId);
  const filtered =
    categories.data?.filter((c) =>
      c.name.toLowerCase().includes(search.toLowerCase().trim()),
    ) ?? [];

  return (
    <div className="categories-page">
      <div className="page-heading">
        <div>
          <p className="eyebrow">A NAME FOR THE EVERYDAY</p>
          <h1>
            A little context.
            <br />
            <em>A clearer story.</em>
          </h1>
          <p className="page-description">
            Organize the details that make your money yours.
          </p>
        </div>
        <button
          className="btn btn-primary new-account-button"
          onClick={() => {
            setNotice("");
            setCreating(true);
          }}
        >
          <Icon name="plus" width="17" height="17" />
          New category
        </button>
      </div>
      <div
        role="status"
        aria-live="polite"
        className={notice ? "success-notice" : "sr-only"}
      >
        {notice && (
          <>
            <Icon name="check" />
            {notice}
          </>
        )}
      </div>
      <section aria-labelledby="category-list-title">
        <div className="section-heading">
          <div>
            <h2 id="category-list-title">
              Your categories
              {categories.isSuccess && (
                <span className="count-badge">{categories.data.length}</span>
              )}
            </h2>
            <p>Shared across all your accounts.</p>
          </div>
          {!!categories.data?.length && (
            <div className="account-search category-search">
              <Icon name="search" width="17" height="17" />
              <input
                aria-label="Search categories"
                placeholder="Find a category…"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
              {search && (
                <button
                  className="icon-button"
                  aria-label="Clear category search"
                  onClick={() => setSearch("")}
                >
                  <Icon name="close" width="15" height="15" />
                </button>
              )}
            </div>
          )}
        </div>
        {categories.isError ? (
          <div role="alert" className="error-notice">
            <span>
              {apiError(categories.error, "Couldn't load your categories.")}
            </span>
            <button
              className="text-button"
              onClick={() => categories.refetch()}
            >
              Try again
            </button>
          </div>
        ) : categories.isPending ? (
          <div
            className="category-grid"
            aria-busy="true"
            aria-label="Loading categories"
          >
            {[0, 1, 2, 3].map((i) => (
              <div className="category-tile" key={i}>
                <span className="skeleton skeleton-icon" />
                <span className="skeleton skeleton-line" />
              </div>
            ))}
          </div>
        ) : categories.data.length === 0 ? (
          <div className="category-empty">
            <span className="account-symbol tone-apricot">
              <Icon name="tag" width="27" height="27" />
            </span>
            <h3>Make the everyday recognizable.</h3>
            <p>
              Groceries, payday, a weekend away.
              <br />
              Start with a category that means something to you.
            </p>
            <button
              className="btn btn-primary"
              onClick={() => setCreating(true)}
            >
              <Icon name="plus" width="17" height="17" />
              Create your first category
            </button>
          </div>
        ) : filtered.length ? (
          <div className="category-grid">
            {filtered.map((c) => (
              <div className="category-tile" key={c.id}>
                <span
                  className={`account-symbol tone-${tones[c.id % tones.length]}`}
                >
                  <Icon name="tag" width="21" height="21" />
                </span>
                <div>
                  <h3>{c.name}</h3>
                  <p>Ready for your entries</p>
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className="search-empty">
            <Icon name="search" width="27" height="27" />
            <h3>No categories found</h3>
            <p>Try a different name.</p>
            <button className="btn" onClick={() => setSearch("")}>
              Clear search
            </button>
          </div>
        )}
      </section>
      <section
        className="suggestions-section"
        aria-labelledby="suggestions-title"
      >
        <div className="section-heading">
          <div>
            <p className="eyebrow">SUGGESTED. NEVER DECIDED FOR YOU.</p>
            <h2 id="suggestions-title">A second look</h2>
            <p>Review suggested categories, one account at a time.</p>
          </div>
          {account && (
            <div className="review-account-picker">
              <label htmlFor="review-account">Review account</label>
              <select
                id="review-account"
                value={account.id}
                onChange={(e) => {
                  setChoice(e.target.value);
                  setSelectedId(null);
                }}
                className="input"
              >
                {accounts.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name} · {a.currency}
                  </option>
                ))}
              </select>
            </div>
          )}
        </div>
        {accountsQuery.isError ? (
          <div className="error-notice" role="alert">
            <span>Couldn't load your accounts.</span>
            <button
              className="text-button"
              onClick={() => accountsQuery.refetch()}
            >
              Try again
            </button>
          </div>
        ) : accountsQuery.isPending || (account && entries.isPending) ? (
          <div
            className="activity-loading card"
            aria-label="Checking suggestions"
            aria-busy="true"
          >
            <span className="skeleton" />
            <span className="skeleton" />
          </div>
        ) : !account ? (
          <div className="suggestions-empty">
            <Icon name="wallet" width="25" height="25" />
            <h3>A place for your first entry.</h3>
            <p>
              Create an account and record a transaction to start adding
              context.
            </p>
            <Link className="text-link" to="/welcome">
              Get started
              <Icon name="arrow" width="15" height="15" />
            </Link>
          </div>
        ) : (
          <div className="suggestions-panel">
            {entries.isError && (
              <div className="error-notice" role="alert">
                <span>
                  Couldn't check all of this activity. Loaded entries are shown
                  below.
                </span>
                <button
                  className="text-button"
                  onClick={() =>
                    entries.isFetchNextPageError
                      ? entries.fetchNextPage()
                      : entries.refetch()
                  }
                >
                  Try again
                </button>
              </div>
            )}
            {suggestions.length ? (
              <ul className="suggestion-list">
                {suggestions.map((entry) => (
                  <li key={entry.posting_id}>
                    <span className="movement-icon income">
                      <Icon name="sparkle" width="19" height="19" />
                    </span>
                    <div className="suggestion-description">
                      <strong>{entry.note || "Transaction"}</strong>
                      <span>
                        {new Date(entry.occurred_at).toLocaleDateString()} ·{" "}
                        {formatMoney(entry.amount_minor, entry.currency)}
                      </span>
                    </div>
                    <span className="suggestion-proposal">
                      <Icon name="tag" width="13" height="13" />
                      {categories.data?.find(
                        (c) => c.id === entry.suggested_category_id,
                      )?.name ?? "Suggested category"}
                    </span>
                    <button
                      className="btn"
                      disabled={categories.isPending || categories.isError}
                      onClick={() => setSelectedId(entry.posting_id)}
                    >
                      Review
                      <Icon name="arrow" width="15" height="15" />
                    </button>
                  </li>
                ))}
              </ul>
            ) : (
              !entries.isError && (
                <div className="suggestions-empty">
                  <span className="account-symbol tone-sage">
                    <Icon name="check" width="25" height="25" />
                  </span>
                  <h3>
                    {entries.hasNextPage
                      ? "Nothing waiting in these entries."
                      : "Nothing waiting for your review."}
                  </h3>
                  <p>
                    {entries.hasNextPage
                      ? "Check older activity for more suggestions."
                      : "Suggestions appear here when a new transaction has one. Your confirmation is always the final step."}
                  </p>
                </div>
              )
            )}
            <div className="suggestions-footer">
              <span>
                {loaded.length}{" "}
                {loaded.length === 1 ? "transaction" : "transactions"} checked
                in {account.name}
              </span>
              {entries.hasNextPage && (
                <button
                  className="text-link"
                  disabled={entries.isFetchingNextPage}
                  onClick={() => entries.fetchNextPage()}
                >
                  {entries.isFetchingNextPage
                    ? "Checking…"
                    : "Check older activity"}
                  <Icon name="down" width="15" height="15" />
                </button>
              )}
            </div>
          </div>
        )}
      </section>
      {creating && (
        <CreateCategoryDialog
          categories={categories.data ?? []}
          onClose={() => setCreating(false)}
          onCreated={(name) => setNotice(`${name} is ready for your entries.`)}
        />
      )}
      {selected && account && categories.data && (
        <SuggestionDialog
          key={selected.posting_id}
          entry={selected}
          accountName={account.name}
          categories={categories.data}
          onClose={() => setSelectedId(null)}
          onConfirmed={() =>
            setNotice(
              "Category confirmed. A little more context, just the way you want it.",
            )
          }
        />
      )}
    </div>
  );
}
