export function isValidMonth(value: string): boolean {
  return /^(?!0000)\d{4}-(0[1-9]|1[0-2])$/.test(value);
}

export function monthLabel(value: string): string {
  if (!isValidMonth(value)) return "Choose a month";
  return new Date(`${value}-01T00:00:00Z`).toLocaleDateString(undefined, {
    month: "long",
    year: "numeric",
    timeZone: "UTC",
  });
}
