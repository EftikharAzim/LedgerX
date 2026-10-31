import type { CategorySuggestionInput } from "../api/endpoints";

// A response belongs only to the draft that requested it. Changing account
// currency, transaction kind, or note must never expose an old Apply action.
export function sameSuggestionDraft(
  requested: CategorySuggestionInput | undefined,
  current: CategorySuggestionInput,
) {
  return (
    !!requested &&
    requested.note === current.note &&
    requested.kind === current.kind &&
    requested.currency === current.currency
  );
}
