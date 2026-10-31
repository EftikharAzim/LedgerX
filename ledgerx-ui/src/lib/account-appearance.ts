export const ACCOUNT_ICONS = ["wallet", "bank", "card", "coins"] as const;
export const ACCOUNT_COLORS = ["sage", "apricot", "lavender", "sky"] as const;
export type AccountAppearance = {
  icon: (typeof ACCOUNT_ICONS)[number];
  color: (typeof ACCOUNT_COLORS)[number];
};

export function readAppearance(id: number): AccountAppearance {
  const fallback: AccountAppearance = {
    icon: ACCOUNT_ICONS[(id - 1) % 4],
    color: ACCOUNT_COLORS[(id - 1) % 4],
  };
  try {
    const saved = JSON.parse(
      localStorage.getItem(`ledgerx.appearance.${id}`) ?? "null",
    );
    if (
      saved &&
      ACCOUNT_ICONS.includes(saved.icon) &&
      ACCOUNT_COLORS.includes(saved.color)
    )
      return saved;
  } catch {
    /* Storage may be unavailable or contain an older preference. */
  }
  return fallback;
}

export function saveAppearance(id: number, appearance: AccountAppearance) {
  try {
    localStorage.setItem(
      `ledgerx.appearance.${id}`,
      JSON.stringify(appearance),
    );
  } catch {
    /* Appearance is optional; account creation must still succeed. */
  }
}
