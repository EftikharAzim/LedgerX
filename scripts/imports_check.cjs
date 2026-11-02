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
  const out = process.env.LEDGERX_QA_OUTPUT || "/tmp/ledgerx-imports-check";
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
  const api = async (method, path, data) => {
    const response = await page.request.fetch(base + path, {
      method,
      headers,
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
  async function chooseCSV(name, csv) {
    await page
      .getByLabel("CSV file", { exact: true })
      .setInputFiles({ name, mimeType: "text/csv", buffer: Buffer.from(csv) });
    await page.getByRole("button", { name: "Continue", exact: true }).click();
    await page
      .getByRole("heading", {
        name: "Make sure the numbers mean the right thing.",
      })
      .waitFor();
  }
  async function preview() {
    await page
      .getByRole("button", { name: "Preview import", exact: true })
      .click();
    await page
      .getByRole("heading", { name: "Choose what to bring in." })
      .waitFor();
  }
  const reviewed = () =>
    page.getByRole("checkbox", {
      name: "I reviewed the selected rows, possible duplicates, and starting balance.",
    });
  try {
    await signIn();
    const account = await api("POST", "/v1/accounts", {
      name: "Bank history",
      currency: "USD",
      opening_balance_minor: 10000,
      opening_balance_at: new Date(Date.now() - 60000).toISOString(),
    });
    await api("POST", "/v1/transactions", {
      account_id: account.id,
      amount_minor: -250,
      currency: "USD",
      occurred_at: "2024-01-04T12:00:00Z",
      note: "Manually recorded",
    });
    const before = await api(
      "GET",
      `/v1/accounts/${account.id}/import-context`,
    );
    assert.equal(before.balance_minor, 9750);
    await page.goto(ui + `/imports?account_id=${account.id}`);
    await page
      .getByRole("heading", { name: "Choose a CSV file", exact: true })
      .waitFor();
    await audit("import-file-desktop");
    const csv =
      "Posted;Debit;Credit;Memo;Category;Currency;Reference\n02/01/2024;12,34;;Coffee;Food;USD;A\n03/01/2024;;100,00;Salary;Income;USD;B\n04/01/2024;2,50;;Already logged;Food;USD;C\n02/01/2024;12,34;;Coffee;Food;USD;A\nbad;3,00;;Invalid;Bad category;USD;D\n05/01/2024;;5,00;Foreign;Food;EUR;E\n";
    await chooseCSV("bank-history.csv", csv);
    // Unfamiliar date header stays unassigned; ambiguous dates are never guessed.
    await page.getByLabel("Date", { exact: true }).selectOption("0");
    assert.equal(
      await page.getByLabel("Date format", { exact: true }).inputValue(),
      "",
    );
    await page
      .getByLabel("Date format", { exact: true })
      .selectOption("day_first");
    await page.getByLabel("Number format in the file").selectOption("comma");
    await page
      .getByRole("radio", { name: "Correct the starting balance or its date" })
      .check();
    await page.getByLabel("Starting balance · USD", { exact: true }).fill("50");
    await page
      .getByLabel("Start date · UTC, optional", { exact: true })
      .fill("2024-01-01");
    await page
      .getByText("See the first 5 source rows", { exact: true })
      .click();
    await audit("import-mapping-desktop");
    await page.setViewportSize({ width: 320, height: 900 });
    await audit("import-mapping-320");
    await page.setViewportSize({ width: 1440, height: 1100 });
    await preview();
    assert.equal(
      await page
        .locator("#review-import-title")
        .evaluate((e) => document.activeElement === e),
      true,
    );
    assert.match(await page.locator(".import-counts").innerText(), /2 ready/);
    assert.match(
      await page.locator(".import-counts").innerText(),
      /2 possible duplicates/,
    );
    assert.match(
      await page.locator(".import-counts").innerText(),
      /2 need correction/,
    );
    assert.equal(
      (await api("GET", `/v1/accounts/${account.id}/import-context`))
        .balance_minor,
      9750,
    );
    assert.equal(
      await page
        .getByRole("checkbox", {
          name: "Include possible duplicate row 4",
          exact: true,
        })
        .isChecked(),
      false,
    );
    await page
      .getByRole("checkbox", {
        name: "Include possible duplicate row 5",
        exact: true,
      })
      .check();
    await page
      .getByRole("button", { name: "Select ready rows", exact: true })
      .click();
    assert.equal(
      await page
        .getByRole("checkbox", {
          name: "Include possible duplicate row 5",
          exact: true,
        })
        .isChecked(),
      false,
    );
    await page
      .getByRole("checkbox", {
        name: "Include possible duplicate row 5",
        exact: true,
      })
      .check();
    await reviewed().check();
    await audit("import-review-desktop");
    await page.setViewportSize({ width: 390, height: 900 });
    await audit("import-review-390");
    await page.setViewportSize({ width: 320, height: 900 });
    await audit("import-review-320");
    await page.setViewportSize({ width: 1440, height: 1100 });
    const importID = Number(new URL(page.url()).searchParams.get("import_id"));
    // Server saves the batch, then its response is lost. The UI recovers the receipt.
    await page.route(
      "**/v1/imports/*/commit",
      async (route) => {
        await route.fetch();
        await route.abort("failed");
      },
      { times: 1 },
    );
    await page
      .getByRole("button", { name: "Import 3 records", exact: true })
      .click();
    await page
      .getByRole("heading", { name: "Your activity is ready." })
      .waitFor();
    const after = await api("GET", `/v1/accounts/${account.id}/import-context`);
    assert.equal(after.balance_minor, 12282);
    assert.equal(after.opening_minor, 5000);
    assert.equal(after.opening_count, 1);
    const receipt = await api("GET", `/v1/imports/${importID}`);
    assert.equal(receipt.receipt.imported_count, 3);
    assert.equal(receipt.receipt.invalid_count, 2);
    assert.equal(receipt.receipt.duplicate_skipped, 1);
    await page.route(
      `**/v1/imports/${importID}`,
      (route) => route.abort("failed"),
      { times: 1 },
    );
    await page.reload();
    await page
      .getByRole("button", { name: "Try loading again", exact: true })
      .click();
    await page
      .getByRole("heading", { name: "Your activity is ready." })
      .waitFor();
    await audit("import-receipt-desktop");
    passed.push(
      "atomic import, duplicate opt-in, baseline correction, lost-response recovery and receipt reload",
    );
    await page
      .getByRole("button", { name: "Check balance", exact: true })
      .click();
    await page.getByRole("dialog").waitFor();
    await page
      .getByLabel("Bank or wallet balance · USD", { exact: true })
      .fill("123.82");
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "Check balance", exact: true })
      .click();
    await page
      .getByRole("heading", { name: "A difference needs review" })
      .waitFor();
    await audit("balance-check-difference-desktop");
    assert.match(
      await page.locator(".balance-check-result").innerText(),
      /\$1\.00 lower/,
    );
    await page
      .getByLabel("Bank or wallet balance · USD", { exact: true })
      .fill("122.82");
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "Check balance", exact: true })
      .click();
    await page
      .getByRole("heading", { name: "Balances matched at this check" })
      .waitFor();
    await page.setViewportSize({ width: 320, height: 900 });
    await audit("balance-check-match-320");
    await page.getByRole("button", { name: "Done", exact: true }).click();
    assert.equal(
      (await api("GET", `/v1/accounts/${account.id}/import-context`))
        .balance_minor,
      12282,
    );
    passed.push("difference and matched snapshots never change the ledger");
    await page.setViewportSize({ width: 1440, height: 1100 });
    await page.getByRole("link", { name: "Review imported activity" }).click();
    await page.locator(".activity-entry-list li").first().waitFor();
    assert.equal(await page.locator(".activity-entry-list li").count(), 3);
    assert.equal(
      await page
        .getByRole("link", { name: "View import receipt", exact: true })
        .count(),
      1,
    );
    const coffee = page
      .locator(".activity-entry-list li")
      .filter({ hasText: "Coffee" })
      .first();
    assert.match(await coffee.locator("time").innerText(), /Jan 2, 2024/);
    await coffee.getByRole("button").click();
    assert.match(await page.getByRole("dialog").innerText(), /Jan 2, 2024/);
    assert.equal(
      await page
        .getByRole("link", { name: `Import #${importID}`, exact: true })
        .count(),
      1,
    );
    await page
      .getByRole("button", { name: "Close dialog", exact: true })
      .click();
    passed.push(
      "import filter, receipt provenance and date-only history in America/Los_Angeles",
    );
    const report = await api("POST", "/v1/exports?type=all");
    let reportStatus;
    for (let i = 0; i < 40; i++) {
      reportStatus = await api("GET", `/v1/exports/${report.id}/status`);
      if (reportStatus.status === "done" || reportStatus.status === "error")
        break;
      await page.waitForTimeout(500);
    }
    assert.equal(
      reportStatus.status,
      "done",
      "background export did not finish",
    );
    const download = await page.request.get(
      base + `/v1/exports/${report.id}/download`,
      { headers },
    );
    assert.equal(download.status(), 200);
    const exportPath = `${out}/import-export.csv`;
    fs.writeFileSync(exportPath, await download.body());
    const exported = JSON.parse(
      execFileSync(
        "python3",
        [
          "-c",
          "import csv,json,sys\nwith open(sys.argv[1], newline='') as stream:\n print(json.dumps(list(csv.DictReader(stream))))",
          exportPath,
        ],
        { encoding: "utf8" },
      ),
    );
    const imported = exported.filter((r) => r.import_id === String(importID));
    assert.equal(imported.length, 3);
    assert(
      imported.every(
        (r) => r.date_only === "true" && r.category_name && r.amount,
      ),
    );
    assert.equal(
      imported.filter(
        (r) =>
          r.note === "Coffee" &&
          r.occurred_at === "2024-01-02T00:00:00Z" &&
          r.amount === "-12.34",
      ).length,
      2,
    );
    passed.push(
      "background worker CSV preserves exact amounts, categories, import provenance and source dates",
    );
    await page.goto(ui + `/accounts/${account.id}`);
    await page
      .getByRole("heading", { name: "Bank history", exact: true })
      .waitFor();
    await page.getByText(/Last balance check matched/).waitFor();
    await api("POST", "/v1/transactions", {
      account_id: account.id,
      amount_minor: 10,
      currency: "USD",
      occurred_at: "2024-01-06T12:00:00Z",
      note: "Later activity",
    });
    await page.reload();
    await page.getByText(/New activity since/).waitFor();
    passed.push("older matched checks are marked stale after new activity");
    await page.goto(ui + `/imports?account_id=${account.id}`);
    await chooseCSV("bank-history.csv", csv);
    await page.getByLabel("Date", { exact: true }).selectOption("0");
    await page
      .getByLabel("Date format", { exact: true })
      .selectOption("day_first");
    await page.getByLabel("Number format in the file").selectOption("comma");
    await preview();
    assert.match(await page.locator(".import-counts").innerText(), /0 ready/);
    await reviewed().check();
    await page
      .getByRole("button", { name: "Finish review", exact: true })
      .click();
    await page.getByRole("heading", { name: "Review completed." }).waitFor();
    const repeated = await api(
      "GET",
      `/v1/accounts/${account.id}/import-context`,
    );
    assert.equal(repeated.balance_minor, 12292);
    assert.equal(repeated.entry_count, after.entry_count + 1);
    passed.push("reimport defaults to zero writes");
    // A first-time workspace can create its destination account inside the flow.
    headers = undefined;
    await signIn();
    await page.goto(ui + "/imports");
    await page
      .getByRole("button", { name: "Create an account", exact: true })
      .click();
    await page.getByLabel("Account name", { exact: true }).fill("Travel yen");
    await page.getByLabel("Currency", { exact: true }).selectOption("JPY");
    await page
      .getByRole("button", { name: "Create account", exact: true })
      .click();
    await page.getByRole("dialog").waitFor({ state: "hidden" });
    await chooseCSV(
      "yen.csv",
      "Date,Amount,Description\n2024-01-02,-100,Lunch\n",
    );
    await preview();
    await reviewed().check();
    await page
      .getByRole("button", { name: "Import 1 record", exact: true })
      .click();
    await page
      .getByRole("heading", { name: "Your activity is ready." })
      .waitFor();
    await page.setViewportSize({ width: 390, height: 900 });
    await audit("import-yen-receipt-390");
    const yenAccounts = await api("GET", "/v1/accounts");
    const yen = yenAccounts.find((a) => a.name === "Travel yen");
    assert.equal(
      (await api("GET", `/v1/accounts/${yen.id}/import-context`)).balance_minor,
      -100,
    );
    passed.push("first account creation, zero opening balance and JPY import");
    // Full-size MoneyLover structure. All content here is invented.
    const daily = await api("POST", "/v1/accounts", {
      name: "History BDT",
      currency: "BDT",
    });
    const travel = await api("POST", "/v1/accounts", {
      name: "Travel USD",
      currency: "USD",
    });
    let history =
      "\ufeffID;Note;Amount;Category;Account;Currency;Date;Event;Exclude Report\n";
    for (let i = 0; i < 6753; i++) {
      history += `history-${i};Synthetic ${i};-1.25;Food;${i === 6752 ? "Travel;USD" : "Daily;BDT"};${i === 6751 ? "23/05/2030" : "23/05/2024"};;${i < 43 ? "True" : "False"}\n`;
    }
    await page.goto(ui + `/imports?account_id=${daily.id}`);
    await page.getByLabel("CSV file", { exact: true }).setInputFiles({
      name: "synthetic-six-years.csv",
      mimeType: "text/csv",
      buffer: Buffer.from(history),
    });
    await page.getByRole("button", { name: "Continue", exact: true }).click();
    await page.getByLabel("Source account and currency").waitFor();
    const sources = await page
      .getByLabel("Source account and currency")
      .locator("option")
      .allTextContents();
    assert(sources.some((s) => s.includes("6,752 records")));
    await page
      .getByLabel("Source account and currency")
      .selectOption({ label: sources.find((s) => s.includes("BDT")) });
    assert.equal(
      await page.getByLabel("Date format", { exact: true }).inputValue(),
      "day_first",
    );
    assert.equal(
      await page.getByLabel(/^Exclude from reports/).inputValue(),
      "8",
    );
    await audit("history-source-routing-desktop");
    await preview();
    assert.equal(await page.locator(".import-rows > li").count(), 100);
    await page.getByRole("button", { name: "Next rows", exact: true }).click();
    assert.match(
      await page
        .getByRole("navigation", { name: "Import review pages" })
        .innerText(),
      /101–200/,
    );
    await page.setViewportSize({ width: 320, height: 900 });
    await audit("history-paged-review-320");
    await reviewed().check();
    await page.getByRole("button", { name: /^Import 6752 records$/ }).click();
    await page
      .getByRole("heading", { name: "Your activity is ready." })
      .waitFor({ timeout: 90000 });
    const fullID = Number(new URL(page.url()).searchParams.get("import_id"));
    const full = (await api("GET", `/v1/imports/${fullID}`)).receipt;
    assert.equal(full.imported_count, 6752);
    assert.equal(full.planned_count, 1);
    assert.equal(full.excluded_report_count, 43);
    assert.equal(
      (await api("GET", `/v1/accounts/${daily.id}/import-context`))
        .balance_minor,
      -6751 * 125,
    );
    await page
      .getByRole("button", { name: "Import another account from this file" })
      .click();
    await page
      .getByLabel("Source account and currency")
      .selectOption({ label: sources.find((s) => s.includes("USD")) });
    await page
      .getByLabel("Destination account", { exact: true })
      .selectOption(String(travel.id));
    await preview();
    await reviewed().check();
    await page.getByRole("button", { name: /^Import 1 record$/ }).click();
    await page
      .getByRole("heading", { name: "Your activity is ready." })
      .waitFor();
    assert.equal(
      (await api("GET", `/v1/accounts/${travel.id}/import-context`))
        .balance_minor,
      -125,
    );
    passed.push(
      "6,753 semicolon/BOM records, two source currencies, 100-row pagination, 43 report exclusions and same-file reuse",
    );
    await page.goto(ui + "/planned");
    await page
      .getByRole("heading", { name: "Synthetic 6751", exact: true })
      .waitFor();
    await audit("planned-list-320");
    await page.getByRole("button", { name: "Record now", exact: true }).click();
    await page.getByLabel("Actual date", { exact: true }).fill("2024-05-24");
    await audit("planned-confirm-320");
    await page
      .getByRole("button", { name: "Confirm & record", exact: true })
      .click();
    await page.getByRole("heading", { name: "Nothing planned yet" }).waitFor();
    assert.equal(
      (await api("GET", `/v1/accounts/${daily.id}/import-context`))
        .balance_minor,
      -6752 * 125,
    );
    passed.push(
      "future import stays outside balance until explicit confirmation of actual date",
    );
    await page.goto(ui + `/accounts/${travel.id}`);
    await page
      .getByRole("button", { name: "Add transaction", exact: true })
      .click();
    await page.getByLabel("Amount", { exact: true }).fill("9.50");
    await page.locator(".entry-date-details summary").click();
    await page
      .getByLabel("Date & time", { exact: true })
      .fill("2030-05-23T12:00");
    await page
      .getByRole("button", { name: "Plan expense", exact: true })
      .click();
    await page.getByRole("dialog").waitFor({ state: "hidden" });
    await page.goto(ui + "/planned");
    await page
      .getByRole("heading", { name: "Planned expense", exact: true })
      .waitFor();
    await page
      .getByRole("button", { name: "Cancel plan", exact: true })
      .click();
    await audit("planned-cancel-320");
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "Cancel plan", exact: true })
      .click();
    await page.getByRole("heading", { name: "Nothing planned yet" }).waitFor();
    assert.equal(
      (await api("GET", `/v1/accounts/${travel.id}/import-context`))
        .balance_minor,
      -125,
    );
    passed.push(
      "manual future expense and cancellation keep the ledger unchanged",
    );
    assert.deepEqual(errors, [], "browser errors");
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
