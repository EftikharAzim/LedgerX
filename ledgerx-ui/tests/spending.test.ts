import assert from "node:assert/strict";
import { test } from "node:test";
import { parseStartingBalance } from "../src/lib/money.ts";
import { entryKind } from "../src/lib/transactions.ts";
import { defaultReportSelection } from "../src/lib/spending.ts";
import { reportRange } from "../src/lib/report-period.ts";

test("starting balances accept signed exact amounts and zero, while preserving currency precision", () => {
  assert.equal(parseStartingBalance("-123.45", "USD"), -12345);
  assert.equal(parseStartingBalance("0.00", "USD"), 0);
  assert.equal(parseStartingBalance("100", "JPY"), 100);
  assert.equal(parseStartingBalance("-1.01", "JPY"), null);
  for (const invalid of ["", "NaN", "1e4", "+10", "1.001", "90071992547409.92"])
    assert.equal(parseStartingBalance(invalid, "USD"), null, invalid);
});

test("refunds and opening money are not labeled as ordinary earnings", () => {
  assert.equal(
    entryKind({ amount_minor: 400, is_transfer: false, entry_type: "refund" }),
    "Refund",
  );
  assert.equal(
    entryKind({
      amount_minor: -100,
      is_transfer: false,
      entry_type: "opening_balance",
    }),
    "Starting balance",
  );
  assert.equal(
    entryKind({
      amount_minor: -400,
      is_transfer: false,
      entry_type: "refund",
      reversal_of: 1,
    }),
    "Reversal",
  );
});

test("default period selections follow explicit UTC report dates at a local month boundary", () => {
  const now = new Date("2026-11-01T00:30:00+06:00");
  assert.deepEqual(defaultReportSelection("monthly", now), {
    type: "monthly",
    month: "2026-10",
  });
  for (const type of ["weekly", "monthly", "yearly", "custom", "all"] as const)
    assert.ok(reportRange(defaultReportSelection(type, now)));
});
