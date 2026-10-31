import assert from "node:assert/strict";
import { test } from "node:test";
import { sameSuggestionDraft } from "../src/lib/category-hint.ts";
import { chooseEntryAccount } from "../src/lib/transactions.ts";
import type { Account } from "../src/api/endpoints.ts";

test("entry defaults follow the displayed currency and ignore inactive or external accounts", () => {
  const account = (
    id: number,
    currency: string,
    active = true,
    kind: Account["kind"] = "normal",
  ): Account => ({ id, currency, active, kind, name: String(id), user_id: 1 });
  const accounts = [
    account(4, "JPY"),
    account(3, "USD"),
    account(1, "USD", false),
    account(2, "USD", true, "external"),
  ];
  assert.equal(chooseEntryAccount(accounts)?.id, 3);
  assert.equal(chooseEntryAccount(accounts, "JPY")?.id, 4);
  assert.equal(chooseEntryAccount(accounts, "USD")?.id, 3);
  assert.equal(chooseEntryAccount([], "USD"), undefined);
});

test("a suggestion can only be applied to the draft that requested it", () => {
  const draft = {
    note: "Morning Coffee",
    currency: "USD",
    kind: "expense" as const,
  };
  assert.equal(sameSuggestionDraft({ ...draft }, draft), true);
  assert.equal(sameSuggestionDraft(undefined, draft), false);
  assert.equal(
    sameSuggestionDraft(draft, { ...draft, note: "Bus fare" }),
    false,
  );
  assert.equal(
    sameSuggestionDraft(draft, { ...draft, currency: "JPY" }),
    false,
  );
  assert.equal(sameSuggestionDraft(draft, { ...draft, kind: "income" }), false);
  assert.equal(sameSuggestionDraft(draft, { ...draft, kind: "refund" }), false);
});
