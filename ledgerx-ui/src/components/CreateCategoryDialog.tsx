import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type Category, createCategory } from "../api/endpoints";
import { apiError } from "../lib/errors";
import Dialog from "./Dialog";
import Icon from "./Icon";

export default function CreateCategoryDialog({
  categories,
  onClose,
  onCreated,
}: {
  categories: Category[];
  onClose: () => void;
  onCreated: (name: string) => void;
}) {
  const [name, setName] = useState("");
  const qc = useQueryClient();
  const duplicate = categories.some(
    (c) => c.name.toLowerCase() === name.trim().toLowerCase(),
  );
  const create = useMutation({
    mutationFn: () => createCategory(name.trim()),
    onSuccess: (category) => {
      void qc.invalidateQueries({ queryKey: ["categories"] });
      onCreated(category.name);
      onClose();
    },
  });
  return (
    <Dialog
      title="A name for the everyday."
      description="One category can bring a little context to many entries."
      onClose={onClose}
      busy={create.isPending}
    >
      <form
        className="account-form"
        onSubmit={(e) => {
          e.preventDefault();
          if (name.trim() && !duplicate && !create.isPending) create.mutate();
        }}
      >
        <div>
          <label className="label" htmlFor="category-name">
            Category name
          </label>
          <input
            id="category-name"
            data-autofocus
            className="input"
            value={name}
            onChange={(e) => {
              setName(e.target.value);
              if (create.isError) create.reset();
            }}
            placeholder="e.g. Everyday essentials"
            required
            disabled={create.isPending}
            aria-invalid={duplicate}
            aria-describedby="category-name-help"
          />
          <p
            id="category-name-help"
            className={duplicate ? "field-error" : "input-hint"}
          >
            {duplicate
              ? "That category is already in your workspace."
              : "Available across all your accounts."}
          </p>
        </div>
        <div>
          <p className="label">Need a little inspiration?</p>
          <div className="category-starters">
            {["Groceries", "Transport", "Salary", "Home"].map((idea) => (
              <button
                type="button"
                className="badge"
                key={idea}
                onClick={() => {
                  setName(idea);
                  create.reset();
                }}
                disabled={create.isPending}
              >
                {idea}
                <Icon name="plus" width="12" height="12" />
              </button>
            ))}
          </div>
        </div>
        {create.isError && (
          <p role="alert" className="form-error">
            <Icon name="info" width="18" height="18" />
            {apiError(create.error)}
          </p>
        )}
        <div className="dialog-actions">
          <button
            type="button"
            className="btn"
            onClick={onClose}
            disabled={create.isPending}
          >
            Cancel
          </button>
          <button
            className="btn btn-primary"
            disabled={!name.trim() || duplicate || create.isPending}
          >
            {create.isPending ? "Creating…" : "Create category"}
            <Icon name="check" width="17" height="17" />
          </button>
        </div>
      </form>
    </Dialog>
  );
}
