import type { Category } from "../api/endpoints";
import type { IconName } from "../components/Icon";

export const categoryIcons: IconName[] = [
  "tag",
  "wallet",
  "home",
  "cart",
  "coffee",
  "car",
  "heart",
  "gift",
  "briefcase",
  "plane",
  "book",
  "food",
];
export function categoryIcon(icon?: string): IconName {
  return categoryIcons.includes(icon as IconName) ? (icon as IconName) : "tag";
}
export function categoryLabel(c: Category, all: Category[]) {
  const p = all.find((x) => x.id === c.parent_id);
  return p ? `${p.name} / ${c.name}` : c.name;
}
export function availableCategory(
  c: Category,
  all: Category[],
  kind?: "income" | "expense" | "refund",
) {
  const p = all.find((x) => x.id === c.parent_id);
  if (c.archived || p?.archived) return false;
  const type = c.kind === "any" || !c.kind ? (p?.kind ?? "any") : c.kind;
  return (
    !kind ||
    type === "any" ||
    type === (kind === "income" ? "income" : "expense")
  );
}
