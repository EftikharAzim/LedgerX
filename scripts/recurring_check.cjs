// Optional real-browser import/reconciliation regression. Creates synthetic data.
// Uses the same Playwright/Chromium/axe configuration as ui_ux_check.cjs.
const { chromium } = require(
  process.env.PLAYWRIGHT_MODULE_PATH || "playwright",
);
const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const fs = require("node:fs");
const { execFileSync } = require("node:child_process");

(async () => {
  const out = process.env.LEDGERX_QA_OUTPUT || "/tmp/ledgerx-recurring-check";
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

  const day = (n) => {
    const at = new Date();
    at.setUTCDate(at.getUTCDate() + n);
    return at.toISOString().slice(0, 10);
  };
  const balance = (id) => api("GET", `/v1/accounts/${id}/import-context`);
  async function saveSeries(name, amount, type, first, end = "") {
    await page
      .getByRole("button", { name: "New schedule", exact: true })
      .click();
    await page.getByLabel("Name", { exact: true }).fill(name);
    await page.getByLabel("Type", { exact: true }).selectOption(type);
    await page
      .getByLabel("Wallet", { exact: true })
      .selectOption({ label: "Bills wallet · USD" });
    await page.getByLabel("Amount", { exact: true }).fill(amount);
    await page
      .getByLabel("Repeat", { exact: true })
      .selectOption(type === "expense" ? "weekly" : "monthly");
    await page.getByLabel("First due date · UTC", { exact: true }).fill(first);
    if (end)
      await page.getByLabel("End date · optional", { exact: true }).fill(end);
    await audit(
      type === "expense"
        ? "recurring-create-desktop"
        : "recurring-income-create-320",
    );
    await page
      .getByRole("button", { name: "Save schedule", exact: true })
      .click();
    await page.getByRole("dialog").waitFor({ state: "hidden" });
    await page.locator(".schedule-card").filter({ hasText: name }).waitFor();
  }
  try {
    await signIn();
    const wallet = await api("POST", "/v1/accounts", {
      name: "Bills wallet",
      currency: "USD",
      opening_balance_minor: 10000,
      opening_balance_at: day(-7) + "T00:00:00Z",
    });
    const foreign = await api("POST", "/v1/accounts", {
      name: "Separate BDT",
      currency: "BDT",
      opening_balance_minor: 5000,
      opening_balance_at: day(-7) + "T00:00:00Z",
    });
    await page.goto(ui + "/schedules");
    await page
      .getByRole("heading", { name: "No recurring schedules", exact: true })
      .waitFor();
    await audit("recurring-empty-desktop");
    await saveSeries("Synthetic weekly bill", "30", "expense", day(-1));
    assert.equal((await balance(wallet.id)).balance_minor, 10000);
    const schedules = await api("GET", "/v1/schedules");
    const bill = schedules.find((s) => s.note === "Synthetic weekly bill");
    const initialPlans = (await api("GET", "/v1/plans")).entries.length;
    assert(initialPlans > 4);
    await page
      .getByRole("link", {
        name: "View upcoming plans & cash flow",
        exact: true,
      })
      .click();
    await page
      .getByText("Projected cash flow · next 30 days", { exact: true })
      .click();
    const forecastWallet = page
      .locator(".cashflow-wallet")
      .filter({ hasText: "Bills wallet" });
    await forecastWallet
      .locator("dl")
      .getByText("-$50.00", { exact: true })
      .waitFor();
    assert.equal(await page.locator(".cashflow-wallet").count(), 2);
    await audit("recurring-projection-desktop");
    await page.setViewportSize({ width: 320, height: 1000 });
    await page
      .getByLabel("Show plans", { exact: true })
      .selectOption("overdue");
    await page.locator(".planned-card").first().waitFor();
    assert.equal(await page.locator(".planned-card").count(), 1);
    await audit("recurring-overdue-320");
    await page.getByRole("button", { name: "Mark paid", exact: true }).click();
    await page.getByLabel("Actual date", { exact: true }).fill(day(0));
    await page.getByLabel("Amount", { exact: true }).fill("31");
    await audit("recurring-confirm-actual-320");
    await page
      .getByRole("button", { name: "Confirm & record", exact: true })
      .click();
    await page
      .getByRole("heading", { name: "No overdue plans", exact: true })
      .waitFor();
    assert.equal((await balance(wallet.id)).balance_minor, 6900);
    await page.getByLabel("Show plans", { exact: true }).selectOption("soon");
    await page.locator(".planned-card").first().waitFor();
    assert.equal(await page.locator(".planned-card").count(), 4);
    await page
      .locator(".planned-card")
      .first()
      .getByRole("button", { name: "Skip occurrence", exact: true })
      .click();
    await audit("recurring-skip-320");
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "Skip occurrence", exact: true })
      .click();
    await page.getByRole("dialog").waitFor({ state: "hidden" });
    assert.equal((await balance(wallet.id)).balance_minor, 6900);
    await api("POST", "/v1/schedules/sync", {});
    assert.equal((await api("GET", "/v1/plans?view=soon")).entries.length, 3);
    passed.push(
      "90-day occurrences stay pending; separate-currency projections, overdue/30-day filters, actual amount/date confirmation and skip are consistent",
    );

    await page.goto(ui + "/schedules");
    await page
      .locator(".schedule-card")
      .filter({ hasText: "Synthetic weekly bill" })
      .getByRole("button", { name: "Pause", exact: true })
      .click();
    await audit("recurring-pause-320");
    await page
      .getByRole("button", { name: "Pause schedule", exact: true })
      .click();
    await page.getByRole("dialog").waitFor({ state: "hidden" });
    assert.equal((await api("GET", "/v1/plans")).entries.length, 0);
    await page
      .locator(".schedule-card")
      .getByRole("button", { name: "Resume", exact: true })
      .click();
    await page
      .getByRole("button", { name: "Resume schedule", exact: true })
      .click();
    await page.getByRole("dialog").waitFor({ state: "hidden" });
    const resumed = (await api("GET", "/v1/plans")).entries;
    assert.equal(resumed.length, initialPlans - 2);
    assert.equal((await balance(wallet.id)).balance_minor, 6900);
    await page
      .locator(".schedule-card")
      .getByRole("button", { name: "Edit schedule", exact: true })
      .click();
    await page.getByLabel("Amount", { exact: true }).fill("35");
    await audit("recurring-edit-320");
    await page
      .getByRole("button", { name: "Save schedule", exact: true })
      .click();
    await page.getByRole("dialog").waitFor({ state: "hidden" });
    assert(
      (await api("GET", "/v1/plans")).entries.every(
        (p) => p.amount_minor === -3500,
      ),
    );
    assert.equal(
      (await api("GET", "/v1/spending?type=all&currency=USD")).spending_minor,
      3100,
    );
    await saveSeries(
      "Synthetic expected income",
      "100",
      "income",
      day(29),
      day(29),
    );
    const flow = await api("GET", "/v1/planning/cashflow");
    const projected = flow.wallets.find((w) => w.account_id === wallet.id);
    assert.equal(projected.projected_minor, 6400);
    assert.equal(projected.lowest_minor, -3600);
    assert.equal(
      flow.wallets.find((w) => w.account_id === foreign.id).current_minor,
      5000,
    );
    await page.goto(ui + "/planned");
    await page
      .getByText("Projected cash flow · next 30 days", { exact: true })
      .click();
    await page.locator(".cashflow-low").waitFor();
    await page
      .locator(".cashflow-wallet")
      .filter({ hasText: "Bills wallet" })
      .getByText("Daily projection", { exact: true })
      .click();
    await audit("recurring-timing-risk-320");
    passed.push(
      "pause/resume preserves skips, edits preserve paid history, and day-end projections reveal a shortfall despite a positive ending estimate",
    );

    await page.goto(ui + "/schedules");
    await page
      .locator(".schedule-card")
      .filter({ hasText: "Synthetic weekly bill" })
      .getByRole("button", { name: "End schedule", exact: true })
      .click();
    await audit("recurring-end-320");
    await page
      .getByRole("button", { name: "Confirm end", exact: true })
      .click();
    await page.getByRole("dialog").waitFor({ state: "hidden" });
    await page.getByLabel("Show ended schedules", { exact: true }).check();
    await page
      .locator(".schedule-card")
      .filter({ hasText: "Synthetic weekly bill" })
      .waitFor();
    assert.equal((await api("GET", "/v1/plans")).entries.length, 1);
    await page.goto(ui + "/planned");
    await page
      .getByRole("button", { name: "Mark received", exact: true })
      .click();
    await page.getByLabel("Actual date", { exact: true }).fill(day(0));
    await page
      .getByRole("button", { name: "Confirm & record", exact: true })
      .click();
    await page
      .getByRole("heading", { name: "Nothing planned yet", exact: true })
      .waitFor();
    assert.equal((await balance(wallet.id)).balance_minor, 16900);
    await page.goto(ui + "/workspace");
    const downloadPromise = page.waitForEvent("download");
    await page
      .getByRole("button", { name: "Download workspace backup", exact: true })
      .click();
    const download = await downloadPromise;
    const file = out + "/recurring-workspace.json";
    await download.saveAs(file);
    const backup = JSON.parse(fs.readFileSync(file, "utf8"));
    assert.equal(backup.data.recurring_schedules.length, 2);
    assert(
      backup.data.planned_transactions.some(
        (p) => p.cancellation_reason === "skipped",
      ),
    );
    assert(
      backup.data.planned_transactions.some(
        (p) => p.recorded_amount_minor === -3100 && p.amount_minor === -3000,
      ),
    );
    for (const field of [
      "creation_key",
      "last_edit_key",
      "creation_hash",
      "last_edit_hash",
    ])
      assert(!JSON.stringify(backup).includes('"' + field + '"'));
    passed.push(
      "ending preserves recorded money; incoming confirmation updates balance; downloadable backup retains schedules, actuals and skipped dates without retry credentials",
    );
    assert.deepEqual(errors, [], "browser errors");
    for (const name of ["failure.png", "failure.txt"])
      fs.rmSync(out + "/" + name, { force: true });
    fs.writeFileSync(
      out + "/results.json",
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
      .screenshot({ path: out + "/failure.png", fullPage: true })
      .catch(() => {});
    fs.writeFileSync(
      out + "/failure.txt",
      error.stack + "\n" + (await page.locator("body").innerText()),
    );
    throw error;
  } finally {
    await browser.close();
  }
})().catch((e) => {
  console.error(e.stack);
  process.exitCode = 1;
});
