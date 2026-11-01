import assert from "node:assert/strict";
import { test } from "node:test";
import {
  shiftMonth,
  groupReportCategories,
} from "../src/lib/financial-reports.ts";
import { availableCategory, categoryLabel } from "../src/lib/categories.ts";

test("month navigation crosses years and respects the supported date boundaries", () => {
  assert.equal(shiftMonth("2024-01", -1), "2023-12");
  assert.equal(shiftMonth("2024-12", 1), "2025-01");
  assert.equal(shiftMonth("0001-01", -1), null);
  assert.equal(shiftMonth("9999-12", 1), null);
  assert.equal(shiftMonth("2024-13", 1), null);
});

test("category rollups include direct entries and child refunds once", () => {
  const cats = [
    { id: 1, user_id: 1, name: "Food" },
    { id: 2, user_id: 1, name: "Groceries", parent_id: 1 },
    { id: 3, user_id: 1, name: "Salary" },
  ];
  const rows = [
    {
      category_id: 1,
      parent_id: 0,
      name: "Food",
      income_minor: 0,
      spending_minor: 100,
      count: 1,
    },
    {
      category_id: 2,
      parent_id: 1,
      name: "Groceries",
      income_minor: 0,
      spending_minor: -20,
      count: 1,
    },
    {
      category_id: 3,
      parent_id: 0,
      name: "Salary",
      income_minor: 1000,
      spending_minor: 0,
      count: 0,
    },
  ];
  const groups = groupReportCategories(rows, cats);
  assert.equal(groups.length, 2);
  assert.equal(groups[0].spending, 80);
  assert.equal(groups[0].children.length, 2);
  assert.equal(groups[1].income, 1000);
  assert.equal(rows[1].spending_minor, -20);
  assert.equal(categoryLabel(cats[1], cats), "Food / Groceries");
});

test("archived parents and category types constrain new-entry choices", () => {
  const parent = {
    id: 1,
    user_id: 1,
    name: "Food",
    kind: "expense",
    archived: true,
  };
  const child = {
    id: 2,
    user_id: 1,
    name: "Groceries",
    parent_id: 1,
    kind: "expense",
  };
  assert.equal(availableCategory(child, [parent, child], "expense"), false);
  const active = { ...parent, archived: false };
  assert.equal(availableCategory(child, [active, child], "refund"), true);
  assert.equal(availableCategory(child, [active, child], "income"), false);
});
