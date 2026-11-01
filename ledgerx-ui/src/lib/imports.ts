import type {
  CSVInspection,
  ImportMapping,
  ImportPreview,
} from "../api/imports.ts";
import { currencyDigits } from "./money.ts";

export const MAX_CSV_BYTES = 16 * 1024 * 1024;

export function decodeCSV(bytes: Uint8Array): string {
  const encoding =
    bytes[0] === 255 && bytes[1] === 254
      ? "utf-16le"
      : bytes[0] === 254 && bytes[1] === 255
        ? "utf-16be"
        : "utf-8";
  try {
    return new TextDecoder(encoding, { fatal: true }).decode(bytes);
  } catch {
    throw new Error(
      "Save this file as UTF-8 CSV or UTF-16 with a byte-order mark.",
    );
  }
}

export function guessMapping(doc: CSVInspection): ImportMapping {
  const names = doc.headers.map((h) =>
    h.toLowerCase().replace(/[_-]/g, " ").replace(/\s+/g, " ").trim(),
  );
  const find = (aliases: string[]) => {
    const matches = names.flatMap((h, i) => (aliases.includes(h) ? [i] : []));
    return matches.length === 1 ? matches[0] : undefined;
  };
  const mapping: ImportMapping = {
    date: find(["date", "transaction date", "posted date", "posting date"]),
    amount: find(["amount", "transaction amount"]),
    debit: find(["debit", "withdrawal", "money out"]),
    credit: find(["credit", "deposit", "money in"]),
    note: find(["note", "description", "details", "memo", "payee"]),
    category: find(["category"]),
    currency: find(["currency"]),
    reference: find(
      doc.source === "moneylover"
        ? ["id"]
        : ["reference", "transaction id", "reference id"],
    ),
    exclude_report: find(["exclude report", "exclude from reports"]),
    amount_mode: "signed",
    date_format:
      doc.date_format === "day_first" || doc.date_format === "month_first"
        ? doc.date_format
        : "",
    number_style: "dot",
    sign_convention: "income_positive",
  };
  if (
    mapping.amount === undefined &&
    (mapping.debit !== undefined || mapping.credit !== undefined)
  )
    mapping.amount_mode = "debit_credit";
  if (
    mapping.date !== undefined &&
    doc.samples.every((r) =>
      /^\d{4}-\d{2}-\d{2}(?:T.*)?$/.test(r[mapping.date!] ?? ""),
    )
  )
    mapping.date_format = "iso";
  const type = find(["type", "transaction type"]);
  if (
    type !== undefined &&
    doc.samples.every((r) =>
      /^(income|expense|refund|debit|credit|deposit|withdrawal|transfer|reversal|opening_balance)$/i.test(
        r[type]?.trim() ?? "",
      ),
    )
  )
    mapping.type = type;
  return mapping;
}

export function mappingError(
  mapping: ImportMapping,
  columns: number,
): string | null {
  if (mapping.date === undefined || !mapping.date_format)
    return "Choose the date column and date format.";
  if (mapping.amount_mode === "signed" && mapping.amount === undefined)
    return "Choose the amount column.";
  if (
    mapping.amount_mode === "debit_credit" &&
    mapping.debit === undefined &&
    mapping.credit === undefined
  )
    return "Choose a debit or credit column.";
  const positions = [
    mapping.date,
    mapping.note,
    mapping.category,
    mapping.currency,
    mapping.type,
    mapping.reference,
    mapping.exclude_report,
    ...(mapping.amount_mode === "signed"
      ? [mapping.amount]
      : [mapping.debit, mapping.credit]),
  ].filter((p): p is number => p !== undefined);
  if (
    new Set(positions).size !== positions.length ||
    positions.some((p) => !Number.isInteger(p) || p < 0 || p >= columns)
  )
    return "Each field needs a different valid column.";
  return null;
}

export function minorInput(minor: number, currency: string): string {
  const value = BigInt(minor),
    digits = currencyDigits(currency),
    sign = value < 0n ? "-" : "";
  const text = (value < 0n ? -value : value)
    .toString()
    .padStart(digits + 1, "0");
  return (
    sign + (digits ? text.slice(0, -digits) + "." + text.slice(-digits) : text)
  );
}

export function readyRows(preview: ImportPreview): number[] {
  return preview.rows
    .filter((r) => !r.errors.length && !r.duplicates.length)
    .map((r) => r.number);
}

// BigInt protects the live preview from rounding when many exact rows are added.
export function importProjection(preview: ImportPreview, selected: number[]) {
  const ids = new Set(selected),
    limit = BigInt(Number.MAX_SAFE_INTEGER);
  const delta = preview.rows
    .filter((r) => ids.has(r.number) && !r.planned)
    .reduce((sum, r) => sum + BigInt(r.amount_minor), 0n);
  let balance = BigInt(preview.context.balance_minor) + delta;
  if (preview.starting_balance.mode === "set")
    balance +=
      BigInt(preview.starting_balance.amount_minor) -
      BigInt(preview.context.opening_minor);
  if (delta > limit || delta < -limit || balance > limit || balance < -limit)
    return null;
  return { delta: Number(delta), balance: Number(balance) };
}
