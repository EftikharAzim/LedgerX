import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import {
  type Account,
  type Entry,
  type Category,
  confirmCategory,
  reverseTransaction,
  formatMoney,
} from "../api/endpoints";
import { apiError } from "../lib/errors";
import { entryKind, entryDate } from "../lib/transactions";
import { Link } from "react-router-dom";
import Dialog from "./Dialog";
import Icon from "./Icon";
import CorrectionDialog from "./CorrectionDialog";

export default function TransactionDrawer({
  entry,
  account,
  counterpartyName,
  categories,
  categoriesError,
  categoriesPending,
  onRetryCategories,
  onClose,
  onReversed,
  onCategorized,
  onRelated,
  relatedAvailable,
}: {
  entry: Entry;
  account: Account;
  counterpartyName?: string;
  categories: Category[];
  categoriesError: boolean;
  categoriesPending: boolean;
  onRetryCategories: () => void;
  onClose: () => void;
  onReversed: (message: string) => void;
  onCategorized: () => Promise<unknown>;
  onRelated: (id: number) => void;
  relatedAvailable: boolean;
}) {
  const [confirmReverse, setConfirmReverse] = useState(false);
  const [editing, setEditing] = useState(false);
  const [category, setCategory] = useState("");
  const [key] = useState(() => crypto.randomUUID());
  const kind = entryKind(entry);
  const reverse = useMutation({
    mutationFn: () => reverseTransaction(entry.transaction_id, key),
    onSuccess: () =>
      onReversed(
        "Transaction reversed. The original and its balancing entry are both in your history.",
      ),
  });
  const categorize = useMutation({
    mutationFn: (id: number) => confirmCategory(entry.transaction_id, id),
    onSuccess: onCategorized,
  });
  const relatedId = entry.reversal_of ?? entry.reversed_by;
  const selectedCategory = category || String(entry.category_id ?? "");
  const canReverse = !entry.reversal_of && !entry.reversed_by;
  if (editing)
    return (
      <CorrectionDialog
        id={entry.transaction_id}
        onClose={() => setEditing(false)}
        onSaved={onReversed}
      />
    );
  return (
    <Dialog
      title={
        confirmReverse ? "Reverse this transaction?" : "Transaction details"
      }
      description={
        confirmReverse
          ? "Review the reversal before confirming."
          : `Transaction #${entry.transaction_id}`
      }
      onClose={onClose}
      busy={reverse.isPending || categorize.isPending}
      drawer
    >
      <div className="drawer-content">
        <div
          className={`entry-detail-amount ${entry.amount_minor > 0 ? "positive" : ""}`}
        >
          <span
            className={`movement-icon ${entry.amount_minor > 0 ? "income" : "expense"}`}
          >
            <Icon
              name={
                entry.reversal_of
                  ? "reverse"
                  : entry.is_transfer
                    ? "transfer"
                    : entry.amount_minor > 0
                      ? "down"
                      : "up"
              }
              width="24"
              height="24"
            />
          </span>
          <strong>
            {entry.amount_minor > 0 ? "+" : ""}
            {formatMoney(entry.amount_minor, entry.currency)}
          </strong>
          <span>
            {kind}
            {entry.reversed_by ? " · Reversed" : ""}
          </span>
        </div>
        {confirmReverse ? (
          <>
            <div className="reversal-explanation">
              <Icon name="reverse" />
              <h3>Reverse this transaction</h3>
              <p>
                A new entry of{" "}
                <strong>
                  {formatMoney(-entry.amount_minor, entry.currency)}
                </strong>{" "}
                will be recorded now in {account.name}.
                {entry.is_transfer &&
                  " The other account in this transfer will also be adjusted."}{" "}
                The original transaction stays in your history.
              </p>
            </div>
            {entry.exclude_from_reports && (
              <p className="input-hint">
                Excluded from income and spending reports. This entry still
                affects your account balance.
              </p>
            )}
            <dl className="review-details">
              <div>
                <dt>Original note</dt>
                <dd>{entry.note || "No note"}</dd>
              </div>
              <div>
                <dt>Originally recorded</dt>
                <dd>{entryDate(entry, true)}</dd>
              </div>
            </dl>
            {reverse.isError && (
              <p role="alert" className="form-error">
                <Icon name="info" />
                {apiError(reverse.error)}
              </p>
            )}
            <div className="dialog-actions">
              <button
                className="btn"
                disabled={reverse.isPending}
                onClick={() => setConfirmReverse(false)}
              >
                Keep transaction
              </button>
              <button
                className="btn btn-caution"
                disabled={reverse.isPending}
                onClick={() => reverse.mutate()}
              >
                {reverse.isPending ? "Reversing…" : "Confirm reversal"}
              </button>
            </div>
          </>
        ) : (
          <>
            <dl className="review-details">
              <div>
                <dt>Account</dt>
                <dd>{account.name}</dd>
              </div>
              {counterpartyName && (
                <div>
                  <dt>Other account</dt>
                  <dd>{counterpartyName}</dd>
                </div>
              )}
              <div>
                <dt>
                  {entry.import_date_only ? "Calendar date" : "Date & time"}
                </dt>
                <dd>{entryDate(entry, true)}</dd>
              </div>
              <div>
                <dt>Currency</dt>
                <dd>{entry.currency}</dd>
              </div>
              <div>
                <dt>Note</dt>
                <dd>{entry.note || "No note added"}</dd>
              </div>
              <div>
                <dt>Reference</dt>
                <dd>TX-{entry.transaction_id}</dd>
              </div>
              {entry.import_id && (
                <div>
                  <dt>Imported from CSV</dt>
                  <dd>
                    <Link
                      className="text-link"
                      to={`/imports?import_id=${entry.import_id}`}
                    >
                      Import #{entry.import_id}
                    </Link>
                  </dd>
                </div>
              )}
            </dl>
            {(entry.correction_of || entry.corrected_by) && (
              <div className="entry-relationship">
                <Icon name="receipt" />
                <div>
                  <strong>
                    {entry.corrected_by
                      ? "Updated by a linked correction"
                      : "Replacement for an earlier record"}
                  </strong>
                  <p>Your original record remains in history.</p>
                  <Link
                    className="text-link"
                    to={`/activity?transaction_id=${entry.corrected_by || entry.correction_of}`}
                  >
                    View {entry.corrected_by ? "replacement" : "original"} #
                    {entry.corrected_by || entry.correction_of}
                  </Link>
                </div>
              </div>
            )}
            {relatedId && (
              <div className="entry-relationship">
                <Icon name="reverse" />
                <div>
                  <strong>
                    {entry.reversal_of ? "Reverses" : "Reversed by"} transaction
                    #{relatedId}
                  </strong>
                  <p>The original and correction are linked.</p>
                  {relatedAvailable && (
                    <button
                      className="text-button"
                      onClick={() => onRelated(relatedId)}
                    >
                      View {entry.reversal_of ? "original" : "reversal"}
                    </button>
                  )}
                </div>
              </div>
            )}
            {!entry.is_transfer &&
              entry.entry_type !== "opening_balance" &&
              !entry.reversal_of &&
              !entry.reversed_by && (
                <section className="drawer-category">
                  <h3>Category</h3>
                  {categoriesPending ? (
                    <span
                      className="skeleton skeleton-line"
                      aria-label="Loading categories"
                    />
                  ) : categoriesError ? (
                    <p role="alert" className="form-error">
                      Couldn't load categories.{" "}
                      <button
                        className="text-button"
                        onClick={onRetryCategories}
                      >
                        Try again
                      </button>
                    </p>
                  ) : (
                    <>
                      {!!entry.suggested_category_id && !entry.category_id && (
                        <div className="category-suggestion">
                          <Icon name="sparkle" width="16" height="16" />
                          <div>
                            <strong>
                              {categories.find(
                                (c) => c.id === entry.suggested_category_id,
                              )?.name ?? "Suggested category"}
                            </strong>
                            <span>A suggestion for you to review.</span>
                          </div>
                          <button
                            className="text-button"
                            disabled={categorize.isPending}
                            onClick={() =>
                              categorize.mutate(entry.suggested_category_id!)
                            }
                          >
                            Accept
                          </button>
                        </div>
                      )}
                      {categories.length ? (
                        <form
                          className="category-select-form"
                          onSubmit={(e) => {
                            e.preventDefault();
                            if (selectedCategory)
                              categorize.mutate(Number(selectedCategory));
                          }}
                        >
                          <label className="sr-only" htmlFor="entry-category">
                            Choose category
                          </label>
                          <select
                            id="entry-category"
                            className="input"
                            value={selectedCategory}
                            disabled={categorize.isPending}
                            onChange={(e) => setCategory(e.target.value)}
                          >
                            <option value="">Choose a category</option>
                            {categories.map((c) => (
                              <option key={c.id} value={c.id}>
                                {c.name}
                              </option>
                            ))}
                          </select>
                          <button
                            className="btn"
                            disabled={
                              !selectedCategory ||
                              Number(selectedCategory) === entry.category_id ||
                              categorize.isPending
                            }
                          >
                            {categorize.isPending ? "Saving…" : "Save"}
                          </button>
                        </form>
                      ) : (
                        <p className="input-hint">
                          No categories yet. Add one under “Manage categories”
                          on your account page.
                        </p>
                      )}
                    </>
                  )}
                  {categorize.isError && (
                    <p role="alert" className="form-error">
                      {apiError(categorize.error)}
                    </p>
                  )}
                  {categorize.isSuccess && (
                    <p className="category-saved" role="status">
                      <Icon name="check" width="15" height="15" />
                      Category saved
                    </p>
                  )}
                </section>
              )}
            <div className="drawer-bottom">
              <Icon name="shield" width="17" height="17" />
              <p>
                Edits keep linked correction history. A reversal cancels a
                record.
              </p>
            </div>
            {canReverse && (
              <button
                className="btn btn-primary edit-transaction-button"
                disabled={categorize.isPending}
                onClick={() => setEditing(true)}
              >
                Edit transaction
                <Icon name="arrow" width="17" height="17" />
              </button>
            )}
            {canReverse && (
              <button
                className="reverse-entry-button"
                onClick={() => setConfirmReverse(true)}
                disabled={categorize.isPending}
              >
                <Icon name="reverse" width="17" height="17" />
                Reverse transaction
              </button>
            )}
          </>
        )}
      </div>
    </Dialog>
  );
}
