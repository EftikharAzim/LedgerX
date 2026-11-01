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

// Starting balances may be zero or negative, unlike income/expense amounts.
export function parseStartingBalance(
  value: string,
  currency: string,
): number | null {
  const digits = currencyDigits(currency);
  const text = value.trim();
  const pattern = digits
    ? new RegExp(`^-?\\d+(?:\\.\\d{1,${digits}})?$`)
    : /^-?\d+$/;
  if (!pattern.test(text)) return null;
  const negative = text.startsWith("-");
  const unsigned = negative ? text.slice(1) : text;
  if (Number(unsigned) === 0) return 0;
  const minor = parseAmount(unsigned, currency);
  return minor === null ? null : negative ? -minor : minor;
}

export function formatMoney(minor: number, currency: string, locale?: string) {
  const digits = currencyDigits(currency);
  const formatter = new Intl.NumberFormat(locale, {
    style: "currency",
    currency,
  });
  if (!Number.isSafeInteger(minor))
    return formatter.format(minor / 10 ** digits);
  const value = BigInt(minor),
    absolute = value < 0n ? -value : value;
  const text = absolute.toString().padStart(digits + 1, "0");
  const decimal =
    (value < 0n ? "-" : "") +
    (digits ? text.slice(0, -digits) + "." + text.slice(-digits) : text);
  // Modern Intl accepts exact decimal strings. TS's older signature still lists
  // only number/bigint; division into a Number would lose cents near MAX_SAFE.
  const exactFormat = formatter.format as unknown as (value: string) => string;
  return exactFormat(decimal);
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
