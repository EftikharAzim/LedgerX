import assert from "node:assert/strict";
import { test } from "node:test";
import {
  correctionDraft,
  correctionKind,
  sameCorrection,
} from "../src/lib/corrections.ts";
import { localDateTime } from "../src/lib/money.ts";
import type { CorrectionDraft } from "../src/api/workspace.ts";

const original: CorrectionDraft = {
  account_id: 1,
  amount_minor: -129,
  currency: "USD",
  entry_type: "standard",
  occurred_at: "2024-01-02T12:34:56.123Z",
  date_only: false,
  note: "Coffee",
  category_id: 2,
};
function fields() {
  return {
    kind: "expense" as const,
    amount: "1.29",
    accountID: 1,
    toAccountID: 0,
    categoryID: 2,
    note: "Coffee",
    dateOnly: false,
    date: "2024-01-02",
    localTime: localDateTime(new Date(original.occurred_at)),
    timeEdited: false,
  };
}
test("editing a note preserves the exact source instant including seconds", () => {
  const input = { ...fields(), note: "Corrected coffee" };
  const draft = correctionDraft(original, input)!;
  assert.equal(draft.occurred_at, original.occurred_at);
  assert.equal(draft.amount_minor, -129);
  assert.equal(draft.note, "Corrected coffee");
  assert.equal(sameCorrection(draft, original), false);
  assert.equal(
    sameCorrection(correctionDraft(original, fields())!, original),
    true,
  );
});
test("calendar-date edits use explicit UTC midnight across local time zones", () => {
  const draft = correctionDraft(
    { ...original, date_only: true },
    { ...fields(), dateOnly: true, date: "2024-03-04" },
  )!;
  assert.equal(draft.occurred_at, "2024-03-04T00:00:00Z");
  assert.equal(draft.date_only, true);
});
test("edits preserve exact currency precision and allow zero only for starting money", () => {
  assert.equal(correctionDraft(original, { ...fields(), amount: "0" }), null);
  assert.equal(
    correctionDraft(original, { ...fields(), amount: "1.001" }),
    null,
  );
  assert.equal(
    correctionDraft(
      { ...original, currency: "JPY" },
      { ...fields(), amount: "1.29" },
    ),
    null,
  );
  assert.equal(
    correctionDraft(
      { ...original, entry_type: "opening_balance" },
      { ...fields(), kind: "opening_balance", amount: "0" },
    )!.amount_minor,
    0,
  );
  assert.equal(
    correctionDraft(
      { ...original, entry_type: "opening_balance" },
      { ...fields(), kind: "opening_balance", amount: "-1.25" },
    )!.amount_minor,
    -125,
  );
  assert.equal(
    correctionDraft(original, { ...fields(), amount: "90071992547409.91" })!
      .amount_minor,
    -Number.MAX_SAFE_INTEGER,
  );
  assert.equal(
    correctionDraft(original, { ...fields(), amount: "90071992547409.92" }),
    null,
  );
});
test("transfer direction, destination, and cleared category stay explicit", () => {
  const transfer = {
    ...original,
    entry_type: "transfer" as const,
    to_account_id: 3,
    category_id: 0,
  };
  assert.equal(correctionKind(transfer), "transfer");
  assert.equal(
    correctionDraft(transfer, {
      ...fields(),
      kind: "transfer",
      toAccountID: 1,
    }),
    null,
  );
  const draft = correctionDraft(transfer, {
    ...fields(),
    kind: "transfer",
    toAccountID: 3,
  })!;
  assert.equal(draft.amount_minor, -129);
  assert.equal(draft.category_id, 0);
  assert.equal(draft.to_account_id, 3);
  assert.equal(
    correctionDraft(original, { ...fields(), kind: "refund" })!.amount_minor,
    129,
  );
  assert.equal(
    correctionDraft(original, { ...fields(), categoryID: 0 })!.category_id,
    0,
  );
});
