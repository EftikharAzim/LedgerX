import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { Category } from "../api/endpoints";
import {
  categoryImpact,
  editCategory,
  deleteCategory,
} from "../api/money-management";
import { availableCategory, categoryLabel } from "../lib/categories";
import { apiError } from "../lib/errors";
import { refreshLedger } from "../lib/refresh-ledger";
import Dialog from "./Dialog";
import CategoryFields from "./CategoryFields";

export default function EditCategoryDialog({
  category,
  categories,
  onClose,
  onSaved,
}: {
  category: Category;
  categories: Category[];
  onClose: () => void;
  onSaved: (message: string) => void;
}) {
  const qc = useQueryClient();
  const [name, setName] = useState(category.name);
  const [meta, setMeta] = useState({
    parent: category.parent_id ?? 0,
    kind: category.kind ?? "any",
    icon: category.icon ?? "tag",
  });
  const [deleting, setDeleting] = useState(false);
  const [replacement, setReplacement] = useState(0);
  const impact = useQuery({
    queryKey: ["category-impact", category.id],
    queryFn: () => categoryImpact(category.id),
    enabled: deleting,
    staleTime: 0,
  });
  const success = async (message: string) => {
    onSaved(message);
    onClose();
    await refreshLedger(qc);
    await qc.invalidateQueries({ queryKey: ["budgets"] });
  };
  const save = useMutation({
    mutationFn: (archive?: boolean) =>
      editCategory(
        category.id,
        archive !== undefined
          ? { archived: archive }
          : {
              name: name.trim(),
              parent_id: meta.parent,
              kind: meta.kind,
              icon: meta.icon,
            },
      ),
    onSuccess: () =>
      success("Category updated. Your recorded money is unchanged."),
  });
  const remove = useMutation({
    mutationFn: () =>
      deleteCategory(category.id, replacement, impact.data!.review_token),
    onSuccess: () =>
      success(
        `Category deleted. Records moved to ${categories.find((c) => c.id === replacement)?.name ?? "Uncategorized"}.`,
      ),
  });
  const busy = save.isPending || remove.isPending;
  return (
    <Dialog
      title={deleting ? "Delete category" : "Edit category"}
      onClose={onClose}
      busy={busy}
    >
      {deleting ? (
        <div className="account-form">
          <p>
            Transactions and plans are kept. Choose where their category labels
            should move.
          </p>
          {impact.isPending ? (
            <p role="status">Checking category usage…</p>
          ) : impact.isError ? (
            <p role="alert">{apiError(impact.error)}</p>
          ) : (
            impact.data && (
              <>
                <p>
                  <strong>{impact.data.transactions}</strong> recorded entries ·{" "}
                  <strong>{impact.data.plans}</strong> plans
                </p>
                {(impact.data.children || impact.data.budgets) ? (
                  <p role="alert" className="form-error">
                    This category has {impact.data.children} subcategories and{" "}
                    {impact.data.budgets} budgets. Move the subcategories and
                    remove or move its budgets first.
                  </p>
                ) : (
                  <>
                    <label className="label" htmlFor="category-replacement">
                      Move records to
                    </label>
                    <select
                      className="input"
                      id="category-replacement"
                      value={replacement}
                      disabled={busy}
                      onChange={(e) => {
                        setReplacement(Number(e.target.value));
                        remove.reset();
                      }}
                    >
                      <option value={0}>Uncategorized</option>
                      {categories
                        .filter(
                          (c) =>
                            c.id !== category.id &&
                            availableCategory(c, categories) &&
                            (category.kind === "any" ||
                              c.kind === "any" ||
                              c.kind === category.kind),
                        )
                        .map((c) => (
                          <option value={c.id} key={c.id}>
                            {categoryLabel(c, categories)}
                          </option>
                        ))}
                    </select>
                    <button
                      className="btn btn-primary"
                      disabled={busy}
                      onClick={() => remove.mutate()}
                    >
                      {remove.isPending ? "Deleting…" : "Delete & move records"}
                    </button>
                  </>
                )}
              </>
            )
          )}
          {remove.isError && (
            <p role="alert" className="form-error">
              {apiError(remove.error)}
            </p>
          )}
          <button
            className="text-button"
            disabled={busy || impact.isFetching}
            onClick={() => {
              remove.reset();
              void impact.refetch();
            }}
          >
            Refresh impact review
          </button>
          <button
            className="btn"
            disabled={busy}
            onClick={() => setDeleting(false)}
          >
            Back to editing
          </button>
        </div>
      ) : (
        <form
          className="account-form"
          onSubmit={(e) => {
            e.preventDefault();
            if (!busy && name.trim()) save.mutate(undefined);
          }}
        >
          <div>
            <label className="label" htmlFor="edit-category-name">
              Category name
            </label>
            <input
              className="input"
              id="edit-category-name"
              value={name}
              maxLength={80}
              required
              disabled={busy}
              onChange={(e) => {
                setName(e.target.value);
                save.reset();
              }}
            />
          </div>
          <CategoryFields
            categories={categories}
            id={category.id}
            parent={meta.parent}
            kind={meta.kind}
            icon={meta.icon}
            onChange={setMeta}
            disabled={busy}
          />
          <p className="input-hint">
            Moving a subcategory changes the parent totals in reports and
            budgets. Archiving a parent also archives its subcategories;
            unarchive each when needed.
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
              disabled={busy}
              onClick={onClose}
            >
              Cancel
            </button>
            <button className="btn btn-primary" disabled={busy || !name.trim()}>
              {save.isPending ? "Saving…" : "Save category"}
            </button>
          </div>
          <button
            className="text-button"
            type="button"
            disabled={busy}
            onClick={() => save.mutate(!category.archived)}
          >
            {category.archived ? "Unarchive category" : "Archive category"}
          </button>
          <p className="input-hint">
            Archived categories retain history and disappear from new-entry
            choices.
          </p>
          <button
            className="text-button"
            type="button"
            disabled={busy}
            onClick={() => setDeleting(true)}
          >
            Delete category…
          </button>
        </form>
      )}
    </Dialog>
  );
}
