// Real-browser category, budget and report regression. Creates synthetic data.
// Uses the same Playwright/Chromium/axe configuration as ui_ux_check.cjs.
const { chromium } = require(
  process.env.PLAYWRIGHT_MODULE_PATH || "playwright",
);
const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const fs = require("node:fs");
const { execFileSync } = require("node:child_process");

(async () => {
  const out = process.env.LEDGERX_QA_OUTPUT || "/tmp/ledgerx-money-check";
  fs.mkdirSync(out, { recursive: true });
  const ui = (process.env.LEDGERX_UI_URL || "http://127.0.0.1:5173").replace(
    /\/$/,
    "",
  );
  const base = (process.env.LEDGERX_API_URL || "http://127.0.0.1:8080").replace(
    /\/$/,
    "",
  );
  const axe =
    process.env.AXE_SCRIPT_PATH || require.resolve("axe-core/axe.min.js");
  const browser = await chromium.launch({
    headless: true,
    ...(process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH
      ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH }
      : {}),
    ...(process.env.PLAYWRIGHT_NO_SANDBOX === "1"
      ? { args: ["--no-sandbox"] }
      : {}),
  });
  const audits = [],
    passed = [],
    errors = [];
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1100 },
    timezoneId: "America/Los_Angeles",
    locale: "en-US",
    reducedMotion: "reduce",
  });
  const page = await context.newPage();
  page.on("pageerror", (e) => errors.push(e.message));
  let headers;
  const api = async (method, path, data, extra = {}) => {
    const response = await page.request.fetch(base + path, {
      method,
      headers: { ...headers, ...extra },
      data,
    });
    assert(
      response.ok(),
      `${method} ${path}: ${response.status()} ${await response.text()}`,
    );
    return response.json();
  };
  async function audit(name) {
    if (!(await page.evaluate(() => !!window.axe)))
      await page.addScriptTag({ path: axe });
    const result = await page.evaluate(async () =>
      axe.run(document, {
        runOnly: {
          type: "tag",
          values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"],
        },
        resultTypes: ["violations"],
      }),
    );
    const violations = result.violations.map((v) => ({
      id: v.id,
      impact: v.impact,
      nodes: v.nodes.map((n) => ({
        target: n.target,
        summary: n.failureSummary,
      })),
    }));
    audits.push({ name, violations });
    assert.deepEqual(violations, [], name + " accessibility");
    assert(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 1,
      ),
      name + " page overflows",
    );
    const overlaps = await page
      .locator(".money-controls")
      .evaluateAll((groups) =>
        groups.flatMap((group) => {
          const controls = [...group.querySelectorAll("input,select,button")]
            .map((el) => ({
              name:
                el.getAttribute("aria-label") || el.id || el.textContent.trim(),
              r: el.getBoundingClientRect(),
            }))
            .filter((x) => x.r.width > 0 && x.r.height > 0);
          return controls.flatMap((a, i) =>
            controls
              .slice(i + 1)
              .filter(
                (b) =>
                  Math.min(a.r.right, b.r.right) -
                    Math.max(a.r.left, b.r.left) >
                    1 &&
                  Math.min(a.r.bottom, b.r.bottom) -
                    Math.max(a.r.top, b.r.top) >
                    1,
              )
              .map((b) => [a.name, b.name]),
          );
        }),
      );
    assert.deepEqual(
      overlaps,
      [],
      name + " overlapping report/budget controls",
    );
    await page.screenshot({ path: `${out}/${name}.png`, fullPage: true });
  }
  async function signIn() {
    const tokens = await api("POST", "/auth/register", {
      email: `imports-${Date.now()}-${crypto.randomBytes(3).toString("hex")}@example.com`,
      password: crypto.randomBytes(18).toString("hex"),
    });
    headers = { Authorization: `Bearer ${tokens.access_token}` };
    await page.goto(ui + "/login");
    await page.evaluate((t) => {
      localStorage.setItem("access_token", t.access_token);
      localStorage.setItem("refresh_token", t.refresh_token);
    }, tokens);
  }

  async function setBudget(category, amount) {
    await page
      .getByRole("button", { name: "Set a budget", exact: true })
      .click();
    await page
      .getByLabel("Budget category", { exact: true })
      .selectOption(String(category));
    await page.getByLabel("Amount", { exact: true }).fill(amount);
    await page
      .getByRole("button", { name: "Save budget", exact: true })
      .click();
    await page.getByRole("dialog").waitFor({ state: "hidden" });
  }
  try {
    await signIn();
    const daily = await api("POST", "/v1/accounts", {
      name: "Everyday wallet",
      currency: "USD",
    });
    const savings = await api("POST", "/v1/accounts", {
      name: "Savings",
      currency: "USD",
    });
    const food = await api("POST", "/v1/categories", {
      name: "Food",
      kind: "expense",
      icon: "food",
    });
    const groceries = await api("POST", "/v1/categories", {
      name: "Groceries",
      parent_id: food.id,
      kind: "expense",
      icon: "cart",
    });
    const salary = await api("POST", "/v1/categories", {
      name: "Salary",
      kind: "income",
      icon: "briefcase",
    });
    const inherited = await api("POST", "/v1/categories", {
      name: "Inherited type",
      parent_id: food.id,
    });
    assert.equal(inherited.kind, "expense");
    for (const [date, amount, category, kind] of [
      ["2023-12-30", 10000, salary.id, "standard"],
      ["2024-01-02", 20000, salary.id, "standard"],
      ["2024-01-03", -4000, groceries.id, "standard"],
      ["2024-01-04", -1000, food.id, "standard"],
      ["2024-01-05", 500, groceries.id, "refund"],
      ["2024-02-02", -800, groceries.id, "standard"],
    ]) {
      await api("POST", "/v1/transactions", {
        account_id: daily.id,
        currency: "USD",
        amount_minor: amount,
        category_id: category,
        entry_type: kind,
        occurred_at: date + "T00:00:00Z",
        note: "Synthetic financial review",
      });
    }
    await api("POST", "/v1/transfers", {
      from_account_id: daily.id,
      to_account_id: savings.id,
      currency: "USD",
      amount_minor: 500,
      occurred_at: "2024-01-06T00:00:00Z",
    });
    const staged = await api("POST", "/v1/imports/preview", {
      account_id: daily.id,
      filename: "synthetic-excluded.csv",
      csv: "Date,Amount,Note,Category,Reference,Exclude Report\n2024-01-07,-7,Excluded,Food,excluded-a,true",
      delimiter: "auto",
      mapping: {
        date: 0,
        amount: 1,
        note: 2,
        category: 3,
        reference: 4,
        exclude_report: 5,
        amount_mode: "signed",
        date_format: "iso",
        number_style: "dot",
        sign_convention: "income_positive",
      },
      starting_balance: { mode: "existing" },
    });
    await api("POST", `/v1/imports/${staged.id}/commit`, {
      rows: [2],
      duplicate_approvals: [],
    });
    await page.goto(ui + "/reports?month=2024-01&currency=USD");
    await page
      .getByRole("heading", { name: "See the story behind your spending." })
      .waitFor();
    await page
      .locator(".financial-stat")
      .filter({ hasText: "Opening balance" })
      .getByText("$100.00", { exact: true })
      .waitFor();
    assert.match(
      await page
        .locator(".financial-stat")
        .filter({ hasText: "Ending balance" })
        .innerText(),
      /248\.00/,
    );
    assert.match(
      await page
        .locator(".financial-stat")
        .filter({ hasText: "Spending" })
        .innerText(),
      /45\.00/,
    );
    await page
      .getByText("Other balance changes: -$7.00", { exact: false })
      .click();
    await audit("financial-report-desktop");
    const foodRow = page
      .locator(".report-category")
      .filter({ hasText: "Food" });
    await foodRow.locator("summary").click();
    assert.match(await foodRow.innerText(), /Groceries/);
    await foodRow.getByRole("link", { name: "View category activity" }).click();
    await page.locator(".activity-row").first().waitFor();
    assert(
      new URL(page.url()).searchParams.get("category_id") === String(food.id),
    );
    await page.goto(ui + "/reports?month=2024-01&currency=USD");
    await page.getByLabel("Breakdown", { exact: true }).selectOption("income");
    await page
      .locator(".report-category")
      .getByText("Salary", { exact: true })
      .waitFor();
    await page.setViewportSize({ width: 320, height: 1000 });
    await audit("financial-report-income-320");
    await page.getByRole("button", { name: "Next report month" }).click();
    assert.equal(
      await page.getByLabel("Month", { exact: true }).inputValue(),
      "2024-02",
    );
    await page.getByRole("button", { name: "Previous report month" }).click();
    await page.getByRole("link", { name: "Download CSV", exact: true }).click();
    assert.equal(
      await page.getByLabel("Month to export", { exact: true }).inputValue(),
      "2024-01",
    );
    await page.goto(ui + "/reports?month=2024-01&currency=USD");
    await page.getByRole("button", { name: /^December 2023 In/ }).click();
    assert.equal(
      await page.getByLabel("Month", { exact: true }).inputValue(),
      "2023-12",
    );
    passed.push(
      "coherent period balances, exclusion reconciliation, income/expense drilldown, parent activity, month navigation and export period handoff",
    );

    await page.goto(ui + "/budgets");
    await page.getByLabel("Budget month", { exact: true }).fill("2024-01");
    await audit("budget-empty-320");
    await setBudget(food.id, "50");
    await page
      .locator(".budget-near")
      .getByText("$5.00", { exact: true })
      .waitFor();
    await setBudget(0, "40");
    await page
      .locator(".budget-over")
      .getByText("$5.00", { exact: true })
      .waitFor();
    await setBudget(groceries.id, "100");
    // Dialog dismissal precedes the query refresh; wait for the saved card,
    // rather than sampling the old list while the new request is in flight.
    await page.locator(".budget-card").nth(2).waitFor();
    assert.equal(await page.locator(".budget-card").count(), 3);
    await audit("budgets-usage-320");
    await page.setViewportSize({ width: 1440, height: 1100 });
    await audit("budgets-usage-desktop");
    await page
      .getByRole("button", { name: "Edit Food budget", exact: true })
      .click();
    await page.getByLabel("Amount", { exact: true }).fill("60");
    await page
      .getByRole("button", { name: "Save budget", exact: true })
      .click();
    await page.getByRole("dialog").waitFor({ state: "hidden" });
    await page.getByRole("button", { name: "Next budget month" }).click();
    await page
      .getByRole("button", { name: "Use last month's budgets", exact: true })
      .click();
    await page.locator(".budget-card").nth(2).waitFor();
    assert.equal(await page.locator(".budget-card").count(), 3);
    const feb = await api("GET", "/v1/budgets?month=2024-02&currency=USD");
    assert.equal(feb.find((b) => b.category_id === food.id).amount_minor, 6000);
    await page.reload();
    await page.getByLabel("Budget month", { exact: true }).fill("2024-02");
    await page.locator(".budget-card").first().waitFor();
    await page
      .locator(".budget-card")
      .filter({ hasText: "Overall monthly budget" })
      .getByRole("button", { name: "Remove budget", exact: true })
      .click();
    await audit("budget-remove-dialog-desktop");
    await page
      .getByRole("button", { name: "Keep budget", exact: true })
      .click();
    passed.push(
      "monthly overall/parent/subcategory limits, refund-aware usage, near/over states, editing, copying and reload persistence",
    );

    await page.goto(ui + "/categories");
    await page
      .getByRole("button", { name: "New category", exact: true })
      .click();
    await page.getByLabel("Category name", { exact: true }).fill("Dining");
    await page.getByText("Subcategory, type & icon", { exact: true }).click();
    await page
      .getByLabel("Parent category", { exact: true })
      .selectOption(String(food.id));
    await page
      .getByRole("button", { name: "coffee icon", exact: true })
      .click();
    await audit("subcategory-create-desktop");
    await page
      .getByRole("button", { name: "Create category", exact: true })
      .click();
    await page.getByRole("dialog").waitFor({ state: "hidden" });
    const dining = (await api("GET", "/v1/categories")).find(
      (c) => c.name === "Dining",
    );
    assert.equal(dining.parent_id, food.id);
    assert.equal(dining.icon, "coffee");
    await page
      .getByRole("button", { name: "Edit Dining category", exact: true })
      .click();
    await page
      .getByRole("button", { name: "Archive category", exact: true })
      .click();
    await page.getByRole("dialog").waitFor({ state: "hidden" });
    await page.getByLabel("Show archived categories", { exact: true }).check();
    await page
      .getByRole("button", { name: "Edit Dining category", exact: true })
      .click();
    await page
      .getByRole("button", { name: "Unarchive category", exact: true })
      .click();
    await page.getByRole("dialog").waitFor({ state: "hidden" });
    await api("POST", "/v1/transactions", {
      account_id: daily.id,
      amount_minor: -200,
      category_id: dining.id,
      currency: "USD",
      occurred_at: "2024-01-08T00:00:00Z",
      note: "Synthetic dinner",
    });
    await api(
      "POST",
      "/v1/plans",
      {
        account_id: daily.id,
        amount_minor: -300,
        category_id: dining.id,
        currency: "USD",
        occurred_at: "2030-01-01T00:00:00Z",
        note: "Synthetic dinner plan",
      },
      { "Idempotency-Key": "synthetic-dinner-plan" },
    );
    const before = await api("GET", `/v1/accounts/${daily.id}/import-context`);
    await page
      .getByRole("button", { name: "Edit Dining category", exact: true })
      .click();
    await page
      .getByRole("button", { name: "Delete category…", exact: true })
      .click();
    await page
      .getByLabel("Move records to", { exact: true })
      .selectOption(String(food.id));
    await page.setViewportSize({ width: 320, height: 1000 });
    await audit("category-reassignment-320");
    await page
      .getByRole("button", { name: "Delete & move records", exact: true })
      .click();
    await page.getByRole("dialog").waitFor({ state: "hidden" });
    assert.equal(
      (await api("GET", `/v1/accounts/${daily.id}/import-context`))
        .balance_minor,
      before.balance_minor,
    );
    assert.equal(
      (await api("GET", "/v1/categories")).some((c) => c.id === dining.id),
      false,
    );
    assert.equal(
      (await api("GET", "/v1/plans")).entries[0].category_id,
      food.id,
    );
    await page
      .getByRole("button", { name: "Edit Groceries category", exact: true })
      .click();
    await page
      .getByRole("button", { name: "Delete category…", exact: true })
      .click();
    await page.getByText(/remove or move its budgets first/).waitFor();
    await audit("category-budget-protection-320");
    await page.keyboard.press("Escape");
    await page.goto(ui + "/reports?month=2024-01&currency=USD");
    await page.getByLabel("Period", { exact: true }).selectOption("custom");
    await page.getByLabel("Start date", { exact: true }).fill("2024-01-01");
    await page.getByLabel("End date", { exact: true }).fill("2024-01-31");
    await page
      .locator(".financial-stat")
      .filter({ hasText: "Spending" })
      .getByText("$47.00", { exact: true })
      .waitFor();
    await audit("financial-custom-report-320");
    passed.push(
      "subcategory and icon creation, archive/unarchive, reviewed deletion reassigns transactions and plans without money changes, budget protection and custom reports",
    );
    await page.route("**/v1/reports/summary?**", (r) =>
      r.fulfill({
        status: 503,
        contentType: "text/plain",
        body: "Report temporarily unavailable",
      }),
    );
    await page.goto(ui + "/reports?month=2001-01&currency=USD");
    await page.getByRole("alert").waitFor();
    await audit("financial-report-error-320");
    await page.unroute("**/v1/reports/summary?**");
    await page.getByRole("button", { name: "Try again", exact: true }).click();
    await page
      .getByText("No spending recorded for this period.", { exact: true })
      .waitFor();
    await audit("financial-report-empty-320");
    passed.push(
      "selectable trend months, empty states, failed report fetch and successful retry",
    );
    assert.deepEqual(errors, [], "browser errors");
    for (const name of ["failure.png", "failure.txt"])
      fs.rmSync(`${out}/${name}`, { force: true });
    fs.writeFileSync(
      `${out}/results.json`,
      JSON.stringify({ passed, audits }, null, 2),
    );
    console.log(
      JSON.stringify(
        { passed, audits: audits.length, violations: 0, output: out },
        null,
        2,
      ),
    );
  } catch (error) {
    await page
      .screenshot({ path: `${out}/failure.png`, fullPage: true })
      .catch(() => {});
    fs.writeFileSync(
      `${out}/failure.txt`,
      `${error.stack}\n${await page.locator("body").innerText()}`,
    );
    throw error;
  } finally {
    await browser.close();
  }
})().catch((e) => {
  console.error(e.stack);
  process.exitCode = 1;
});
