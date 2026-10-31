export function currencyDigits(currency: string): number {
  return (
    new Intl.NumberFormat("en", {
      style: "currency",
      currency,
    }).resolvedOptions().maximumFractionDigits ?? 2
  );
}

// The ledger stores integer minor units. Parse the decimal string directly so
// floating point rounding cannot change a customer's entered amount.
export function parseAmount(value: string, currency: string): number | null {
  const digits = currencyDigits(currency);
  const pattern = digits
    ? new RegExp(`^\\d+(?:\\.\\d{1,${digits}})?$`)
    : /^\d+$/;
  const text = value.trim();
  if (!pattern.test(text)) return null;
  const [whole, fraction = ""] = text.split(".");
  const minor =
    BigInt(whole) * 10n ** BigInt(digits) +
    BigInt(fraction.padEnd(digits, "0") || "0");
  if (minor <= 0n || minor > BigInt(Number.MAX_SAFE_INTEGER)) return null;
  return Number(minor);
}

export function formatMoney(minor: number, currency: string) {
  return new Intl.NumberFormat(undefined, {
    style: "currency",
    currency,
  }).format(minor / 10 ** currencyDigits(currency));
}

export function localDateTime(date = new Date()) {
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
}

export function toOccurredAt(value: string): string | null {
  if (!value) return null;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? null : date.toISOString();
}
