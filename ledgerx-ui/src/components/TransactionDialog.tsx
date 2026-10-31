import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  type Account,
  type Category,
  createTransaction,
  createCategory,
  listCategories,
  formatMoney,
} from "../api/endpoints";
import { localDateTime, parseAmount, toOccurredAt } from "../lib/money";
import { nextWriteAttempt, type WriteAttempt } from "../lib/write-attempt";
import { apiError } from "../lib/errors";
import { readAppearance } from "../lib/account-appearance";
import AmountInput from "./AmountInput";
import Dialog from "./Dialog";
import Icon from "./Icon";

export default function TransactionDialog({
  account,
  onClose,
  onSaved,
}: {
  account: Account;
  onClose: () => void;
  onSaved: (message: string) => void;
}) {
  const [kind, setKind] = useState<"expense" | "income">("expense");
  const [amount, setAmount] = useState("");
  const [note, setNote] = useState("");
  const [date, setDate] = useState(() => localDateTime());
  const [category, setCategory] = useState("");
  const [newCategory, setNewCategory] = useState("");
  const [creatingCategory, setCreatingCategory] = useState(false);
  const qc = useQueryClient();
  const categories = useQuery({
    queryKey: ["categories"],
    queryFn: listCategories,
  });
  const duplicate = categories.data?.some(
    (c) => c.name.toLowerCase() === newCategory.trim().toLowerCase(),
  );
  const addCategory = useMutation({
    mutationFn: () => createCategory(newCategory.trim()),
    onSuccess: (created) => {
      qc.setQueryData<Category[]>(["categories"], (previous = []) => [
        ...previous.filter((c) => c.id !== created.id),
        created,
      ]);
      void qc.invalidateQueries({ queryKey: ["categories"] });
      setCategory(String(created.id));
      setNewCategory("");
      setCreatingCategory(false);
    },
  });
  const attempt = useRef<WriteAttempt | null>(null);
  const minor = parseAmount(amount, account.currency);
  const occurredAt = toOccurredAt(date);
  const appearance = readAppearance(account.id);
  const save = useMutation({
    mutationFn: () => {
      if (!minor || !occurredAt)
        throw new Error("Check the amount and date before saving.");
      const body = {
        account_id: account.id,
        amount_minor: kind === "expense" ? -minor : minor,
        currency: account.currency,
        occurred_at: occurredAt,
        note: note.trim(),
        ...(category ? { category_id: Number(category) } : {}),
      };
      attempt.current = nextWriteAttempt(attempt.current, body);
      return createTransaction(body, attempt.current.key);
    },
    onSuccess: () =>
      onSaved(
        `${kind === "expense" ? "Expense" : "Income"} of ${formatMoney(minor!, account.currency)} recorded.`,
      ),
  });
  return (
    <Dialog
      title="Record a transaction"
      description="Small entries. A clearer picture."
      onClose={onClose}
      busy={save.isPending || addCategory.isPending}
    >
      <form
        className="account-form"
        onChange={() => {
          if (save.isError) save.reset();
        }}
        onSubmit={(e) => {
          e.preventDefault();
          if (
            minor &&
            occurredAt &&
            !save.isPending &&
            !addCategory.isPending &&
            !creatingCategory &&
            !categories.isPending
          )
            save.mutate();
        }}
      >
        <fieldset className="transaction-kind" disabled={save.isPending}>
          <legend className="sr-only">Transaction type</legend>
          {(["expense", "income"] as const).map((type) => (
            <label key={type}>
              <input
                type="radio"
                name="transaction-kind"
                checked={kind === type}
                onChange={() => setKind(type)}
              />
              <span>
                <Icon
                  name={type === "income" ? "down" : "up"}
                  width="17"
                  height="17"
                />
                {type === "income" ? "Income" : "Expense"}
              </span>
            </label>
          ))}
        </fieldset>
        <AmountInput
          value={amount}
          onChange={setAmount}
          currency={account.currency}
          disabled={save.isPending}
        />
        <div className="form-account">
          <span className={`account-symbol tone-${appearance.color}`}>
            <Icon name={appearance.icon} />
          </span>
          <div>
            <span>{kind === "income" ? "Into account" : "From account"}</span>
            <strong>{account.name}</strong>
          </div>
          <span className="account-currency">{account.currency}</span>
        </div>
        <div>
          <label htmlFor="transaction-date" className="label">
            Date & time
          </label>
          <input
            id="transaction-date"
            className="input"
            type="datetime-local"
            required
            value={date}
            onChange={(e) => setDate(e.target.value)}
            disabled={save.isPending}
          />
          <p className="input-hint">In your local time zone.</p>
        </div>
        <div>
          <label htmlFor="transaction-note" className="label">
            Note <span className="optional-label">optional</span>
          </label>
          <input
            id="transaction-note"
            className="input"
            placeholder={
              kind === "income"
                ? "e.g. Monthly salary"
                : "e.g. Coffee with a friend"
            }
            value={note}
            onChange={(e) => setNote(e.target.value)}
            disabled={save.isPending}
          />
        </div>
        <div className="transaction-category">
          <div className="transaction-category-label">
            <label className="label" htmlFor="transaction-category">
              Category <span className="optional-label">optional</span>
            </label>
            <button
              type="button"
              className="text-link"
              disabled={save.isPending || addCategory.isPending}
              onClick={() => setCreatingCategory(!creatingCategory)}
            >
              {creatingCategory ? "Cancel new category" : "New category"}
              <Icon name="plus" width="14" height="14" />
            </button>
          </div>
          <select
            id="transaction-category"
            className="input"
            value={category}
            onChange={(e) => setCategory(e.target.value)}
            disabled={
              save.isPending || addCategory.isPending || categories.isPending
            }
          >
            <option value="">
              {categories.isPending ? "Loading categories…" : "No category"}
            </option>
            {categories.data?.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </select>
          {categories.isError && (
            <p className="form-error" role="alert">
              Couldn't load categories.{" "}
              <button
                type="button"
                className="text-button"
                onClick={() => categories.refetch()}
              >
                Try again
              </button>
            </p>
          )}
          {!categories.isPending &&
            !categories.isError &&
            !categories.data?.length && (
              <p className="input-hint">
                Create your first category here, or leave this entry
                uncategorized.
              </p>
            )}
          {creatingCategory && (
            <div className="inline-category-create">
              <label className="label" htmlFor="inline-category-name">
                New category name
              </label>
              <div>
                <input
                  id="inline-category-name"
                  className="input"
                  value={newCategory}
                  onChange={(e) => {
                    setNewCategory(e.target.value);
                    addCategory.reset();
                  }}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault();
                      if (
                        newCategory.trim() &&
                        !duplicate &&
                        !save.isPending &&
                        !addCategory.isPending
                      )
                        addCategory.mutate();
                    }
                  }}
                  placeholder="e.g. Groceries"
                  disabled={save.isPending || addCategory.isPending}
                />
                <button
                  type="button"
                  className="btn"
                  disabled={
                    !newCategory.trim() ||
                    duplicate ||
                    save.isPending ||
                    addCategory.isPending
                  }
                  onClick={() => addCategory.mutate()}
                >
                  {addCategory.isPending ? "Adding…" : "Add category"}
                </button>
              </div>
              {duplicate && (
                <p className="field-error">
                  That name already exists. Choose it above.
                </p>
              )}
              {addCategory.isError && (
                <p role="alert" className="form-error">
                  {apiError(addCategory.error)}
                </p>
              )}
            </div>
          )}
        </div>
        {save.isError && (
          <p role="alert" className="form-error">
            <Icon name="info" />
            {apiError(save.error)} You can retry this entry safely.
          </p>
        )}
        <div className="dialog-actions">
          <button
            type="button"
            className="btn"
            onClick={onClose}
            disabled={save.isPending || addCategory.isPending}
          >
            Cancel
          </button>
          <button
            className="btn btn-primary"
            disabled={
              !minor ||
              !occurredAt ||
              save.isPending ||
              addCategory.isPending ||
              creatingCategory ||
              categories.isPending
            }
          >
            {save.isPending ? "Recording…" : `Record ${kind}`}
            <Icon name="check" width="17" height="17" />
          </button>
        </div>
      </form>
    </Dialog>
  );
}
