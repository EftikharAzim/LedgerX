import type { Category } from "../api/endpoints";
import { categoryIcons } from "../lib/categories";
import Icon from "./Icon";

export default function CategoryFields({
  categories,
  id = 0,
  parent,
  kind,
  icon,
  onChange,
  disabled = false,
}: {
  categories: Category[];
  id?: number;
  parent: number;
  kind: string;
  icon: string;
  onChange: (value: { parent: number; kind: string; icon: string }) => void;
  disabled?: boolean;
}) {
  const hasChildren = categories.some((c) => c.parent_id === id);
  return (
    <fieldset className="category-fields" disabled={disabled}>
      <legend className="sr-only">Category organization</legend>
      <div>
        <label className="label" htmlFor="category-parent">
          Parent category
        </label>
        <select
          className="input"
          id="category-parent"
          value={parent}
          disabled={hasChildren}
          onChange={(e) => {
            const p = categories.find((c) => c.id === Number(e.target.value));
            onChange({
              parent: Number(e.target.value),
              kind: p?.kind && p.kind !== "any" ? p.kind : kind,
              icon,
            });
          }}
        >
          <option value={0}>None — main category</option>
          {categories
            .filter((c) => !c.parent_id && c.id !== id && !c.archived)
            .map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
        </select>
        <p className="input-hint">
          {hasChildren
            ? "Move the subcategories before making this a subcategory."
            : "One subcategory level keeps entry and reporting simple."}
        </p>
      </div>
      <div>
        <label className="label" htmlFor="category-kind">
          Used for
        </label>
        <select
          className="input"
          id="category-kind"
          value={kind}
          onChange={(e) => onChange({ parent, kind: e.target.value, icon })}
        >
          <option value="any">Income & expenses</option>
          <option value="expense">Expenses & refunds</option>
          <option value="income">Income</option>
        </select>
      </div>
      <div>
        <span className="label">Icon</span>
        <div
          className="category-icon-options"
          role="group"
          aria-label="Category icon"
        >
          {categoryIcons.map((name) => (
            <button
              className="btn"
              aria-label={`${name} icon`}
              aria-pressed={icon === name}
              type="button"
              key={name}
              onClick={() => onChange({ parent, kind, icon: name })}
            >
              <Icon name={name} />
            </button>
          ))}
        </div>
      </div>
    </fieldset>
  );
}
