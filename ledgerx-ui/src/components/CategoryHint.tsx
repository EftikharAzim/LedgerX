import { useMutation } from "@tanstack/react-query";
import {
  suggestCategory,
  type Category,
  type CategorySuggestionInput,
} from "../api/endpoints";
import { sameSuggestionDraft } from "../lib/category-hint";

export default function CategoryHint({
  note,
  currency,
  kind,
  categories,
  disabled,
  onChoose,
}: CategorySuggestionInput & {
  categories: Category[];
  disabled: boolean;
  onChoose: (id: number) => void;
}) {
  const request = useMutation({ mutationFn: suggestCategory });
  const draft = { note: note.trim(), currency, kind };
  const current = sameSuggestionDraft(request.variables, draft);
  const hint = current ? request.data : undefined;
  const category = hint?.category_id
    ? categories.find((c) => c.id === hint.category_id)
    : undefined;
  const valid = [...draft.note].length >= 3 && [...draft.note].length <= 240;

  return (
    <div className="category-hint">
      <div className="category-hint-request">
        <button
          type="button"
          className="text-button"
          disabled={disabled || request.isPending || !valid}
          onClick={() => request.mutate(draft)}
        >
          {request.isPending ? "Checking…" : "Suggest category"}
        </button>
        <span>
          {valid
            ? "Uses your previous choices and category names"
            : "Add a short note to get a suggestion"}
        </span>
      </div>
      {current && request.isError && (
        <p role="status" className="input-hint">
          Couldn't get a suggestion. You can still choose a category.
        </p>
      )}
      {hint && (
        <div className="category-hint-result" role="status">
          <div>
            {category && <strong>{category.name}</strong>}
            <p>{hint.reason}</p>
          </div>
          {category && (
            <button
              type="button"
              className="btn"
              disabled={disabled}
              onClick={() => {
                if (sameSuggestionDraft(request.variables, draft))
                  onChoose(category.id);
              }}
            >
              Use {category.name}
            </button>
          )}
        </div>
      )}
    </div>
  );
}
