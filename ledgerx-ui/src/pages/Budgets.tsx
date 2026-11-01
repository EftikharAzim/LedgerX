import { useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { formatMoney, listAccounts, listCategories } from "../api/endpoints";
import {
  listBudgets,
  saveBudget,
  deleteBudget,
  copyBudgets,
  type Budget,
} from "../api/money-management";
import { availableCategory, categoryLabel } from "../lib/categories";
import { reportRange } from "../lib/report-period";
import { isValidMonth, monthLabel } from "../lib/month";
import { shiftMonth } from "../lib/financial-reports";
import { parseAmount } from "../lib/money";
import { minorInput } from "../lib/imports";
import { apiError } from "../lib/errors";
import Dialog from "../components/Dialog";
import AmountInput from "../components/AmountInput";

export default function Budgets() {
  const [month, setMonth] = useState(() =>
    new Date().toISOString().slice(0, 7),
  );
  const accounts = useQuery({ queryKey: ["accounts"], queryFn: listAccounts });
  const categories = useQuery({
    queryKey: ["categories"],
    queryFn: listCategories,
  });
  const [code, setCode] = useState("");
  const currencies = [
    ...new Set(
      accounts.data?.filter((a) => a.kind === "normal").map((a) => a.currency),
    ),
  ].sort();
  const currency = code || currencies[0] || "USD";
  const budgets = useQuery({
    queryKey: ["budgets", month, currency],
    queryFn: () => listBudgets(month, currency),
    enabled: isValidMonth(month),
  });
  const [editing, setEditing] = useState<Budget | "new" | null>(null);
  const [removing, setRemoving] = useState<Budget | null>(null);
  const [notice, setNotice] = useState("");
  const qc = useQueryClient();
  const refresh = async (message: string) => {
    setNotice(message);
    await qc.invalidateQueries({ queryKey: ["budgets"] });
  };
  const remove = useMutation({
    mutationFn: () => deleteBudget(removing!.id),
    onSuccess: async () => {
      setRemoving(null);
      await refresh("Budget removed. Your transactions are unchanged.");
    },
  });
  const copy = useMutation({
    mutationFn: () => copyBudgets(shiftMonth(month, -1)!, month, currency),
    onSuccess: (r) =>
      refresh(`${r.added} budgets copied. Existing limits were kept.`),
  });
  return (
    <div className="budgets-page">
      <div className="page-heading">
        <div>
          <p className="eyebrow">ROOM FOR WHAT MATTERS</p>
          <h1>Your month, with a plan.</h1>
          <p className="page-description">
            Set a few useful limits. See what you have spent and what remains.
          </p>
        </div>
        <button
          className="btn btn-primary"
          disabled={
            !isValidMonth(month) || categories.isPending || categories.isError
          }
          onClick={() => setEditing("new")}
        >
          Set a budget
        </button>
      </div>
      <div className="money-controls">
        <label>
          Budget month
          <input
            className="input"
            type="month"
            min="0001-01"
            max="9999-12"
            value={month}
            onChange={(e) => {
              setMonth(e.target.value);
              setNotice("");
              copy.reset();
            }}
          />
        </label>
        <div className="money-control">
          <label htmlFor="budget-currency">Currency</label>
          <select
            id="budget-currency"
            className="input"
            value={currency}
            onChange={(e) => {
              setCode(e.target.value);
              copy.reset();
            }}
          >
            {(currencies.length ? currencies : ["USD"]).map((c) => (
              <option key={c}>{c}</option>
            ))}
          </select>
        </div>
        <div className="import-actions">
          <button
            className="btn"
            aria-label="Previous budget month"
            disabled={!shiftMonth(month, -1)}
            onClick={() => setMonth(shiftMonth(month, -1)!)}
          >
            ←
          </button>
          <button
            className="btn"
            aria-label="Next budget month"
            disabled={!shiftMonth(month, 1)}
            onClick={() => setMonth(shiftMonth(month, 1)!)}
          >
            →
          </button>
        </div>
      </div>
      {notice && (
        <p role="status" className="success-notice">
          {notice}
        </p>
      )}
      {copy.isError && (
        <p role="alert" className="form-error">
          {apiError(copy.error)}
        </p>
      )}
      <section aria-labelledby="budget-list-title">
        <div className="section-heading">
          <h2 id="budget-list-title">{monthLabel(month)}</h2>
          <button
            className="text-button"
            disabled={copy.isPending || !shiftMonth(month, -1)}
            onClick={() => copy.mutate()}
          >
            {copy.isPending ? "Copying…" : "Use last month's budgets"}
          </button>
        </div>
        <p className="input-hint">
          Actual spending includes refunds and subcategories. Plans are
          separate. Overall, parent and subcategory limits can overlap; their
          remaining amounts are never added together.
        </p>
        {!isValidMonth(month) ? (
          <p role="alert">Choose a valid month.</p>
        ) : budgets.isPending ? (
          <p role="status">Loading budgets…</p>
        ) : budgets.isError ? (
          <p role="alert" className="form-error">
            {apiError(budgets.error)}{" "}
            <button className="text-button" onClick={() => budgets.refetch()}>
              Try again
            </button>
          </p>
        ) : !budgets.data.length ? (
          <div className="import-panel">
            <h3>Start with one useful limit</h3>
            <p>
              Try an overall monthly budget, or a category you want to keep an
              eye on.
            </p>
            <button className="btn" onClick={() => setEditing("new")}>
              Create your first budget
            </button>
          </div>
        ) : (
          <div className="budget-grid">
            {budgets.data.map((b) => {
              const percent = Math.min(
                100,
                Math.max(0, (b.spent_minor / b.amount_minor) * 100),
              );
              const state =
                b.remaining_minor < 0
                  ? "over"
                  : percent >= 80
                    ? "near"
                    : "within";
              return (
                <article key={b.id} className={`budget-card budget-${state}`}>
                  <div className="section-heading">
                    <h3>
                      {categories.data
                        ? b.category_id
                          ? categoryLabel(
                              categories.data.find(
                                (c) => c.id === b.category_id,
                              ) ?? {
                                id: b.category_id,
                                name: b.category_name,
                                user_id: 0,
                              },
                              categories.data,
                            )
                          : b.category_name
                        : b.category_name}
                    </h3>
                    <button
                      className="text-button"
                      aria-label={`Edit ${b.category_name} budget`}
                      onClick={() => setEditing(b)}
                    >
                      Edit
                    </button>
                  </div>
                  <p className="budget-remaining">
                    <strong>
                      {formatMoney(Math.abs(b.remaining_minor), currency)}
                    </strong>{" "}
                    {b.remaining_minor < 0 ? "over budget" : "remaining"}
                  </p>
                  <progress
                    max={b.amount_minor}
                    value={Math.max(0, Math.min(b.spent_minor, b.amount_minor))}
                    aria-label={`${b.category_name} budget usage`}
                  />
                  <p>
                    {formatMoney(b.spent_minor, currency)} spent of{" "}
                    {formatMoney(b.amount_minor, currency)}
                  </p>
                  <p className="input-hint">
                    {b.archived ? "Archived category · " : ""}
                    {state === "over"
                      ? "Over the limit"
                      : state === "near"
                        ? "Near the limit"
                        : "Within the limit"}
                    {b.parent_id
                      ? " · Subcategory limit"
                      : b.category_id
                        ? " · Includes subcategories"
                        : " · All spending in this currency"}
                  </p>
                  <div className="import-actions">
                    <Link
                      className="text-link"
                      to={`/activity?${new URLSearchParams({ currency, start_date: `${month}-01`, end_date: reportRange({ type: "monthly", month })!.end!, ...(b.category_id ? { category_id: String(b.category_id) } : {}) })}`}
                    >
                      View activity
                    </Link>
                    <button
                      className="text-button"
                      onClick={() => {
                        remove.reset();
                        setRemoving(b);
                      }}
                    >
                      Remove budget
                    </button>
                  </div>
                </article>
              );
            })}
          </div>
        )}
      </section>
      <p className="input-hint">
        <Link to="/planned">Review upcoming plans</Link> before committing the
        rest. A budget is a spending limit, not a forecast of your bank balance.
      </p>
      {editing && (
        <BudgetDialog
          key={`${month}-${currency}-${editing === "new" ? "new" : editing.id}`}
          budget={editing === "new" ? undefined : editing}
          month={month}
          currency={currency}
          categories={categories.data ?? []}
          onClose={() => setEditing(null)}
          onSaved={async () => {
            setEditing(null);
            await refresh("Monthly budget saved.");
          }}
        />
      )}
      {removing && (
        <Dialog
          title="Remove this budget?"
          onClose={() => setRemoving(null)}
          busy={remove.isPending}
        >
          <p>
            {removing.category_name} · {monthLabel(removing.month)}. Your
            recorded money and plans will be kept.
          </p>
          {remove.isError && (
            <p role="alert" className="form-error">
              {apiError(remove.error)}
            </p>
          )}
          <div className="dialog-actions">
            <button
              className="btn"
              disabled={remove.isPending}
              onClick={() => setRemoving(null)}
            >
              Keep budget
            </button>
            <button
              className="btn btn-primary"
              disabled={remove.isPending}
              onClick={() => remove.mutate()}
            >
              Remove budget
            </button>
          </div>
        </Dialog>
      )}
    </div>
  );
}

function BudgetDialog({
  budget,
  month,
  currency,
  categories,
  onClose,
  onSaved,
}: {
  budget?: Budget;
  month: string;
  currency: string;
  categories: Awaited<ReturnType<typeof listCategories>>;
  onClose: () => void;
  onSaved: () => Promise<void>;
}) {
  const [category, setCategory] = useState(budget?.category_id ?? 0);
  const [amount, setAmount] = useState(
    budget ? minorInput(budget.amount_minor, currency) : "",
  );
  const minor = parseAmount(amount, currency);
  const save = useMutation({
    mutationFn: () =>
      saveBudget({
        id: budget?.id,
        month,
        currency,
        category_id: category,
        amount_minor: minor!,
      }),
    onSuccess: onSaved,
  });
  return (
    <Dialog
      title={budget ? "Edit monthly budget" : "Set a monthly budget"}
      onClose={onClose}
      busy={save.isPending}
    >
      <form
        className="account-form"
        onSubmit={(e) => {
          e.preventDefault();
          if (minor && !save.isPending) save.mutate();
        }}
      >
        <p>
          {monthLabel(month)} · {currency}
        </p>
        <div>
          <label className="label" htmlFor="budget-category">
            Budget category
          </label>
          <select
            className="input"
            id="budget-category"
            value={category}
            disabled={save.isPending}
            onChange={(e) => setCategory(Number(e.target.value))}
          >
            <option value={0}>Overall monthly spending</option>
            {categories
              .filter(
                (c) =>
                  availableCategory(c, categories, "expense") ||
                  c.id === budget?.category_id,
              )
              .map((c) => (
                <option key={c.id} value={c.id}>
                  {categoryLabel(c, categories)}
                  {c.archived ? " (archived)" : ""}
                </option>
              ))}
          </select>
        </div>
        <AmountInput
          value={amount}
          onChange={(v) => {
            setAmount(v);
            save.reset();
          }}
          currency={currency}
          disabled={save.isPending}
        />
        <p className="input-hint">
          This limit does not move or reserve money. You can change it anytime.
        </p>
        {save.isError && (
          <p role="alert" className="form-error">
            {apiError(save.error)}
          </p>
        )}
        <div className="dialog-actions">
          <button
            type="button"
            className="btn"
            disabled={save.isPending}
            onClick={onClose}
          >
            Cancel
          </button>
          <button
            className="btn btn-primary"
            disabled={!minor || save.isPending}
          >
            {save.isPending ? "Saving…" : "Save budget"}
          </button>
        </div>
      </form>
    </Dialog>
  );
}
