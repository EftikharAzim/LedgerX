import assert from "node:assert/strict";
import { test } from "node:test";
import {
  guessMapping,
  mappingError,
  minorInput,
  readyRows,
  importProjection,
  decodeCSV,
} from "../src/lib/imports.ts";
import { entryDate } from "../src/lib/transactions.ts";
import type { ImportPreview, ImportRow } from "../src/api/imports.ts";

test("MoneyLover mapping preserves references, report exclusions and inferred dates", () => {
  const m = guessMapping({
    headers: [
      "ID",
      "Note",
      "Amount",
      "Category",
      "Account",
      "Currency",
      "Date",
      "Event",
      "Exclude Report",
    ],
    samples: [
      [
        "ref-1",
        "Synthetic",
        "-1",
        "Food",
        "Daily",
        "BDT",
        "23/05/2024",
        "",
        "True",
      ],
    ],
    source: "moneylover",
    date_format: "day_first",
    delimiter: "semicolon",
    row_count: 6753,
  });
  assert.equal(m.date_format, "day_first");
  assert.equal(m.reference, 0);
  assert.equal(m.exclude_report, 8);
  assert.equal(mappingError(m, 9), null);
});

test("CSV decoding accepts Unicode BOMs and rejects broken text", () => {
  const text = "Date;Note\n2024-01-01;সঞ্চয়";
  assert.equal(decodeCSV(new Uint8Array(Buffer.from("\ufeff" + text))), text);
  const le = Buffer.concat([
    Buffer.from([255, 254]),
    Buffer.from(text, "utf16le"),
  ]);
  assert.equal(decodeCSV(new Uint8Array(le)), text);
  assert.equal(decodeCSV(new Uint8Array(Buffer.from(le).swap16())), text);
  assert.throws(() => decodeCSV(new Uint8Array([255, 255])), /UTF-8/);
});

test("column zero is recognized, duplicate header meanings stay unassigned, ambiguous dates require a choice", () => {
  const doc = {
    headers: ["Date", "Amount", "Description", "Reference"],
    samples: [["03/04/2024", "-1.25", "Coffee", "ABC"]],
    delimiter: "comma",
    row_count: 1,
  };
  const mapping = guessMapping(doc);
  assert.equal(mapping.date, 0);
  assert.equal(mapping.date_format, "");
  assert.equal(mapping.reference, 3);
  assert.match(mappingError(mapping, 4)!, /date format/);
  assert.equal(
    guessMapping({ ...doc, headers: ["Date", "Amount", "Amount", "Reference"] })
      .amount,
    undefined,
  );
  const iso = guessMapping({
    ...doc,
    samples: [["2024-03-04", "-1", "Coffee", "ABC"]],
  });
  assert.equal(iso.date_format, "iso");
  assert.equal(mappingError(iso, 4), null);
  assert.match(mappingError({ ...iso, note: 0 }, 4)!, /different/);
});

test("debit and credit files map separately without using the same column twice", () => {
  const m = guessMapping({
    headers: ["Date", "Debit", "Credit", "Description"],
    samples: [["2024-01-02", "1.25", "", "Coffee"]],
    delimiter: "comma",
    row_count: 1,
  });
  assert.equal(m.amount_mode, "debit_credit");
  assert.equal(m.debit, 1);
  assert.equal(m.credit, 2);
  assert.equal(mappingError(m, 4), null);
  assert.match(mappingError({ ...m, credit: 1 }, 4)!, /different/);
});

function preview(): ImportPreview {
  return {
    context: {
      account_id: 1,
      name: "Bank",
      currency: "USD",
      active: true,
      balance_minor: 10000,
      revision: 1,
      entry_count: 1,
      opening_id: 1,
      opening_minor: 10000,
      opening_count: 1,
      token: "test",
    },
    rows: [
      { number: 2, amount_minor: -100, errors: [], duplicates: [] },
      {
        number: 3,
        amount_minor: -100,
        errors: [],
        duplicates: [{ row_number: 2, match_type: "file" }],
      },
      { number: 4, amount_minor: 0, errors: ["invalid"], duplicates: [] },
    ].map(
      (r) =>
        ({
          occurred_at: "2024-01-01T00:00:00Z",
          date_only: true,
          currency: "USD",
          entry_type: "standard",
          note: "Coffee",
          category: "Food",
          source_ref: "",
          fingerprint: "test",
          new_category: false,
          ...r,
        }) as ImportRow,
    ),
    starting_balance: { mode: "set", amount_minor: 5000, date: "2024-01-01" },
    mapping: {
      amount_mode: "signed",
      date_format: "iso",
      number_style: "dot",
      sign_convention: "income_positive",
    },
    new_categories: [],
    plan_errors: [],
    projected_balance_minor: 4900,
    ready_count: 1,
    duplicate_count: 1,
    invalid_count: 1,
  };
}

test("defaults skip duplicates and invalid rows; live projection corrects only the starting component", () => {
  const p = preview();
  assert.deepEqual(readyRows(p), [2]);
  assert.deepEqual(importProjection(p, [2]), { delta: -100, balance: 4900 });
  assert.deepEqual(importProjection(p, [2, 3]), { delta: -200, balance: 4800 });
  p.starting_balance.mode = "existing";
  assert.deepEqual(importProjection(p, []), { delta: 0, balance: 10000 });
});

test("projection uses integer arithmetic and refuses unsafe combined balances", () => {
  const p = preview();
  p.starting_balance.mode = "existing";
  p.context.balance_minor = Number.MAX_SAFE_INTEGER;
  p.rows[0].amount_minor = 1;
  assert.equal(importProjection(p, [2]), null);
  p.context.balance_minor = 0;
  p.rows[0].amount_minor = Number.MAX_SAFE_INTEGER;
  p.rows[1].amount_minor = -Number.MAX_SAFE_INTEGER;
  assert.deepEqual(importProjection(p, [2, 3]), { delta: 0, balance: 0 });
  assert.equal(minorInput(-123, "USD"), "-1.23");
  assert.equal(minorInput(123, "JPY"), "123");
  assert.equal(minorInput(Number.MAX_SAFE_INTEGER, "USD"), "90071992547409.91");
});

test("date-only imports retain bank calendar dates in western and eastern time zones", () => {
  const original = process.env.TZ;
  try {
    for (const zone of ["America/Los_Angeles", "Asia/Dhaka"]) {
      process.env.TZ = zone;
      const source = entryDate(
        { occurred_at: "2024-03-04T00:00:00Z", import_date_only: true },
        true,
      );
      assert.equal(
        source,
        new Date("2024-03-04T00:00:00Z").toLocaleDateString(undefined, {
          day: "numeric",
          month: "short",
          year: "numeric",
          timeZone: "UTC",
        }),
      );
    }
  } finally {
    if (original === undefined) delete process.env.TZ;
    else process.env.TZ = original;
  }
});
