import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type Category, createCategory } from "../api/endpoints";
import { apiError } from "../lib/errors";
import Icon from "./Icon";

export default function CategoryManager({
  categories,
  pending,
  error,
  onRetry,
}: {
  categories: Category[];
  pending: boolean;
  error: boolean;
  onRetry: () => void;
}) {
  const [name, setName] = useState("");
  const qc = useQueryClient();
  const create = useMutation({
    mutationFn: () => createCategory(name.trim()),
    onSuccess: () => {
      setName("");
      return qc.invalidateQueries({ queryKey: ["categories"] });
    },
  });
  return (
    <details className="category-manager">
      <summary>
        <span>
          <Icon name="list" width="17" height="17" />
          Manage categories
        </span>
        <Icon name="chevron" width="16" height="16" />
      </summary>
      <div className="category-manager-content">
        <p>
          Make your history easier to recognize. Categories are shared across
          your accounts.
        </p>
        {pending ? (
          <span
            className="skeleton skeleton-line"
            aria-label="Loading categories"
          />
        ) : error ? (
          <p role="alert" className="form-error">
            Couldn't load categories.{" "}
            <button className="text-button" onClick={onRetry}>
              Try again
            </button>
          </p>
        ) : (
          <div className="category-badges">
            {categories.map((c) => (
              <span key={c.id} className="badge">
                {c.name}
              </span>
            ))}
            {!categories.length && (
              <span className="input-hint">
                No categories yet. Start with something like Groceries or
                Salary.
              </span>
            )}
          </div>
        )}
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (name.trim() && !create.isPending) create.mutate();
          }}
        >
          <label className="sr-only" htmlFor="new-category">
            New category name
          </label>
          <input
            id="new-category"
            className="input"
            placeholder="New category name"
            value={name}
            disabled={create.isPending}
            onChange={(e) => {
              setName(e.target.value);
              if (create.isError) create.reset();
            }}
          />
          <button className="btn" disabled={!name.trim() || create.isPending}>
            {create.isPending ? "Adding…" : "Add category"}
          </button>
        </form>
        {create.isError && (
          <p role="alert" className="form-error">
            {apiError(create.error)}
          </p>
        )}
        {create.isSuccess && (
          <p role="status" className="category-saved">
            Category added
          </p>
        )}
      </div>
    </details>
  );
}
