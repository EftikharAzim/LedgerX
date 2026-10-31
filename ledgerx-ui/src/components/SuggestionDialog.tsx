import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  type Category,
  type Entry,
  confirmCategory,
  formatMoney,
} from "../api/endpoints";
import { apiError } from "../lib/errors";
import Dialog from "./Dialog";
import Icon from "./Icon";

export default function SuggestionDialog({
  entry,
  accountName,
  categories,
  onClose,
  onConfirmed,
}: {
  entry: Entry;
  accountName: string;
  categories: Category[];
  onClose: () => void;
  onConfirmed: () => void;
}) {
  const [category, setCategory] = useState(
    categories.some((c) => c.id === entry.suggested_category_id)
      ? String(entry.suggested_category_id)
      : "",
  );
  const qc = useQueryClient();
  const confirm = useMutation({
    mutationFn: () => confirmCategory(entry.transaction_id, Number(category)),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["entries"] });
      onConfirmed();
      onClose();
    },
  });
  return (
    <Dialog
      title="A suggestion. Your choice."
      description="Categories are only applied when you confirm them."
      onClose={onClose}
      busy={confirm.isPending}
    >
      <form
        className="account-form"
        onSubmit={(e) => {
          e.preventDefault();
          if (category && !confirm.isPending) confirm.mutate();
        }}
      >
        <div className="suggestion-review">
          <span className="movement-icon expense">
            <Icon name="receipt" />
          </span>
          <div>
            <strong>{entry.note || "Transaction"}</strong>
            <span>
              {accountName} · {new Date(entry.occurred_at).toLocaleDateString()}
            </span>
          </div>
          <strong>{formatMoney(entry.amount_minor, entry.currency)}</strong>
        </div>
        <div>
          <label className="label" htmlFor="suggested-category">
            Category
          </label>
          <select
            id="suggested-category"
            data-autofocus
            className="input"
            value={category}
            disabled={confirm.isPending}
            onChange={(e) => {
              setCategory(e.target.value);
              confirm.reset();
            }}
          >
            <option value="">Choose a category</option>
            {categories.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </select>
          <p className="input-hint">
            Keep the suggestion, or choose a category that fits better.
          </p>
        </div>
        <p className="transfer-explanation">
          <Icon name="info" width="16" height="16" />
          This adds context to your entry. Its amount and account balance stay
          the same.
        </p>
        {confirm.isError && (
          <p role="alert" className="form-error">
            <Icon name="info" width="18" height="18" />
            {apiError(confirm.error)}
          </p>
        )}
        <div className="dialog-actions">
          <button
            type="button"
            className="btn"
            onClick={onClose}
            disabled={confirm.isPending}
          >
            Leave for later
          </button>
          <button
            className="btn btn-primary"
            disabled={!category || confirm.isPending}
          >
            {confirm.isPending ? "Confirming…" : "Confirm category"}
            <Icon name="check" width="17" height="17" />
          </button>
        </div>
      </form>
    </Dialog>
  );
}
