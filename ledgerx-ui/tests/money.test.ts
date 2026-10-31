import assert from "node:assert/strict";
import { test } from "node:test";
import {
  currencyDigits,
  formatMoney,
  localDateTime,
  parseAmount,
  toOccurredAt,
} from "../src/lib/money.ts";
import { nextWriteAttempt } from "../src/lib/write-attempt.ts";
import { entryKind } from "../src/lib/transactions.ts";

test("decimal entry converts to exact minor units without floating-point rounding", () => {
  assert.equal(parseAmount("1.01", "USD"), 101);
  assert.equal(parseAmount("0.29", "EUR"), 29);
  assert.equal(parseAmount("00012.5", "BDT"), 1250);
  assert.equal(
    parseAmount("90071992547409.91", "USD"),
    Number.MAX_SAFE_INTEGER,
  );
  assert.equal(parseAmount("90071992547409.92", "USD"), null);
});

test("amount entry rejects signs, zero, exponent notation and excessive precision", () => {
  for (const value of [
    "",
    "0",
    "0.00",
    "-12",
    "+12",
    "1e3",
    "1.001",
    "NaN",
    "Infinity",
    "1,000.00",
    ".",
    "1.",
  ]) {
    assert.equal(parseAmount(value, "USD"), null, value);
  }
});

test("JPY amounts are entered and displayed in whole yen", () => {
  assert.equal(currencyDigits("JPY"), 0);
  assert.equal(parseAmount("1250", "JPY"), 1250);
  assert.equal(parseAmount("12.50", "JPY"), null);
  assert.equal(
    formatMoney(1250, "JPY"),
    new Intl.NumberFormat(undefined, {
      style: "currency",
      currency: "JPY",
    }).format(1250),
  );
});

test("date entry round-trips the local minute and rejects missing or invalid dates", () => {
  const original = new Date("2026-10-31T19:00:00.000Z");
  assert.equal(toOccurredAt(localDateTime(original)), original.toISOString());
  assert.equal(toOccurredAt(""), null);
  assert.equal(toOccurredAt("not a date"), null);
});

test("retrying a write reuses its key; editing the payload creates a new intent", () => {
  const body = { account_id: 1, amount_minor: -101, currency: "USD" };
  const first = nextWriteAttempt(null, body);
  assert.equal(nextWriteAttempt(first, { ...body }), first);
  const changed = nextWriteAttempt(first, { ...body, amount_minor: -102 });
  assert.notEqual(changed.key, first.key);
  assert.notEqual(nextWriteAttempt(null, body).key, first.key);
});

test("history labels use ledger relationships rather than a user-controlled note", () => {
  assert.equal(
    entryKind({ amount_minor: 1200, is_transfer: true }),
    "Transfer",
  );
  assert.equal(
    entryKind({ amount_minor: -1200, is_transfer: true, reversal_of: 10 }),
    "Reversal",
  );
  assert.equal(
    entryKind({ amount_minor: -1200, is_transfer: false }),
    "Expense",
  );
  assert.equal(entryKind({ amount_minor: 1200, is_transfer: false }), "Income");
});
