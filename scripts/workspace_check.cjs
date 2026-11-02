// Five synthetic workflow scenarios. These are regression checks, not customer interviews.
// Optional tools/configuration match ui_ux_check.cjs; Python 3 parses real exports.
const { chromium } = require(
  process.env.PLAYWRIGHT_MODULE_PATH || "playwright",
);
const assert = require("node:assert/strict"),
  crypto = require("node:crypto"),
  fs = require("node:fs");
const { execFileSync } = require("node:child_process");
(async () => {
  const out = process.env.LEDGERX_QA_OUTPUT || "/tmp/ledgerx-workspace-check";
  fs.mkdirSync(out, { recursive: true });
  const ui = (process.env.LEDGERX_UI_URL || "http://127.0.0.1:5173").replace(
      /\/$/,
      "",
    ),
    base = (process.env.LEDGERX_API_URL || "http://127.0.0.1:8080").replace(
      /\/$/,
      "",
    );
  const browser = await chromium.launch({
    headless: true,
    ...(process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH
      ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH }
      : {}),
    ...(process.env.PLAYWRIGHT_NO_SANDBOX === "1"
      ? { args: ["--no-sandbox"] }
      : {}),
  });
  const context = await browser.newContext({
      viewport: { width: 1440, height: 1100 },
      timezoneId: "America/Los_Angeles",
      locale: "en-US",
      reducedMotion: "reduce",
    }),
    page = await context.newPage();
  const errors = [],
    passed = [],
    audits = [];
  page.on("pageerror", (e) => errors.push(e.message));
  let headers;
  async function api(method, path, data) {
    const r = await page.request.fetch(base + path, { method, headers, data });
    assert(r.ok(), `${method} ${path}: ${r.status()} ${await r.text()}`);
    return r.json();
  }
  async function audit(name) {
    if (!(await page.evaluate(() => !!window.axe)))
      await page.addScriptTag({
        path:
          process.env.AXE_SCRIPT_PATH || require.resolve("axe-core/axe.min.js"),
      });
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
      name + " page overflow",
    );
    await page.screenshot({ path: `${out}/${name}.png`, fullPage: true });
  }
  async function openEdit(id) {
    await page.goto(ui + `/activity?transaction_id=${id}`);
    await page.locator(".activity-entry-list li").first().waitFor();
    await page
      .locator(".activity-entry-list li")
      .first()
      .getByRole("button")
      .click();
    await page
      .getByRole("button", { name: "Edit transaction", exact: true })
      .click();
    await page
      .getByRole("heading", { name: "Edit transaction", exact: true })
      .waitFor();
    await page.getByLabel(/Amount ·|Starting balance ·/).waitFor();
  }
  async function preview() {
    await page
      .getByRole("button", { name: "Review correction", exact: true })
      .click();
    await page
      .getByRole("heading", { name: "Review correction", exact: true })
      .waitFor();
  }
  async function save() {
    await page
      .getByRole("checkbox", {
        name: "I reviewed this correction and its balance changes.",
      })
      .check();
    await page
      .getByRole("button", { name: "Save correction", exact: true })
      .click();
    await page.getByRole("dialog").waitFor({ state: "hidden" });
  }
  const state = (id) => api("GET", `/v1/accounts/${id}/import-context`);
  try {
    const tokens = await api("POST", "/auth/register", {
      email: `workspace-${Date.now()}@example.com`,
      password: crypto.randomBytes(18).toString("hex"),
    });
    headers = { Authorization: `Bearer ${tokens.access_token}` };
    const sourceHeaders = { ...headers };
    await page.goto(ui + "/login");
    await page.evaluate((t) => {
      localStorage.setItem("access_token", t.access_token);
      localStorage.setItem("refresh_token", t.refresh_token);
    }, tokens);
    const bank = await api("POST", "/v1/accounts", {
        name: "Everyday bank",
        currency: "USD",
        opening_balance_minor: 10000,
        opening_balance_at: "2024-01-01T00:00:00Z",
      }),
      cash = await api("POST", "/v1/accounts", {
        name: "Cash wallet",
        currency: "USD",
      }),
      yen = await api("POST", "/v1/accounts", {
        name: "Travel yen",
        currency: "JPY",
        opening_balance_minor: 10000,
        opening_balance_at: "2024-01-01T00:00:00Z",
      });
    const food = await api("POST", "/v1/categories", { name: "Food" });
    await api("POST", "/v1/transactions", {
      account_id: bank.id,
      amount_minor: 100000,
      currency: "USD",
      occurred_at: "2024-01-02T12:00:00Z",
      note: "Salary",
    });
    const coffee = await api("POST", "/v1/transactions", {
      account_id: bank.id,
      amount_minor: -129,
      currency: "USD",
      occurred_at: "2024-01-02T12:34:56.123Z",
      note: "Coffee raw",
      category_id: food.id,
    });
    const beforeBank = await state(bank.id);
    await openEdit(coffee.id);
    await page.getByLabel("Amount · USD", { exact: true }).fill("2.34");
    await page
      .getByRole("dialog")
      .getByLabel("Account", { exact: true })
      .selectOption(String(cash.id));
    await page
      .getByLabel("Note · optional", { exact: true })
      .fill("Coffee corrected");
    await page
      .getByLabel("Category · optional", { exact: true })
      .selectOption("0");
    await audit("edit-desktop");
    await page.setViewportSize({ width: 320, height: 900 });
    await audit("edit-320");
    await page.setViewportSize({ width: 1440, height: 1100 });
    await preview();
    await audit("correction-review-desktop");
    await page.setViewportSize({ width: 390, height: 900 });
    await audit("correction-review-390");
    await page.setViewportSize({ width: 1440, height: 1100 });
    await page.route(
      "**/v1/transactions/*/corrections",
      async (route) => {
        await route.fetch();
        await route.abort("failed");
      },
      { times: 1 },
    );
    await save();
    const original = (
      await api("GET", `/v1/activity?transaction_id=${coffee.id}`)
    ).entries[0];
    assert(original.corrected_by && original.reversed_by);
    const changed = (
      await api("GET", `/v1/activity?transaction_id=${original.corrected_by}`)
    ).entries[0];
    assert.equal(changed.account_id, cash.id);
    assert.equal(changed.amount_minor, -234);
    assert.equal(changed.category_id, undefined);
    assert.equal(changed.occurred_at, "2024-01-02T12:34:56.123Z");
    assert.equal(changed.correction_of, coffee.id);
    assert.equal(
      (await state(bank.id)).balance_minor,
      beforeBank.balance_minor + 129,
    );
    assert.equal((await state(cash.id)).balance_minor, -234);
    // Refreshing an outdated preview is explicit and cannot post a partial edit.
    await openEdit(changed.transaction_id);
    await page.getByLabel("Amount · USD", { exact: true }).fill("3.00");
    await preview();
    await api("POST", "/v1/transactions", {
      account_id: cash.id,
      amount_minor: 10,
      currency: "USD",
      occurred_at: "2024-01-03T12:00:00Z",
      note: "Later cash entry",
    });
    await page
      .getByRole("checkbox", {
        name: "I reviewed this correction and its balance changes.",
      })
      .check();
    await page
      .getByRole("button", { name: "Save correction", exact: true })
      .click();
    await page.getByRole("alert").filter({ hasText: "changed" }).waitFor();
    await page
      .getByRole("button", { name: "Back to editing", exact: true })
      .click();
    await preview();
    await save();
    assert.equal((await state(cash.id)).balance_minor, -290);
    passed.push(
      "Daily tracking: edit amount/account/note/category, exact timestamps, lost-response recovery and stale-review retry",
    );
    const spendingBefore = await api(
      "GET",
      "/v1/spending?type=all&currency=USD",
    );
    const transfer = await api("POST", "/v1/transfers", {
      from_account_id: bank.id,
      to_account_id: cash.id,
      amount_minor: 500,
      currency: "USD",
      occurred_at: "2024-01-04T12:00:00Z",
      note: "Cash withdrawal",
    });
    await openEdit(transfer.id);
    await page.getByLabel("Amount · USD", { exact: true }).fill("7.50");
    await preview();
    await save();
    const spendingAfter = await api(
      "GET",
      "/v1/spending?type=all&currency=USD",
    );
    assert.equal(spendingAfter.spending_minor, spendingBefore.spending_minor);
    assert.equal(spendingAfter.income_minor, spendingBefore.income_minor);
    assert.equal((await state(cash.id)).balance_minor, 460);
    passed.push(
      "Cash tracking: correcting a transfer changes both balances without increasing spending or income",
    );
    const refund = await api("POST", "/v1/transactions", {
      account_id: bank.id,
      amount_minor: 100,
      currency: "USD",
      occurred_at: "2024-01-05T12:00:00Z",
      note: "Partial return",
      entry_type: "refund",
      category_id: food.id,
    });
    await openEdit(refund.id);
    await page.getByLabel("Amount · USD", { exact: true }).fill("1.50");
    await preview();
    await save();
    const lunch = await api("POST", "/v1/transactions", {
      account_id: yen.id,
      amount_minor: -250,
      currency: "JPY",
      occurred_at: "2024-01-06T12:00:00Z",
      note: "Lunch in Tokyo",
    });
    await openEdit(lunch.id);
    await page.getByLabel("Amount · JPY", { exact: true }).fill("1.50");
    assert.equal(
      await page
        .getByRole("button", { name: "Review correction", exact: true })
        .isDisabled(),
      true,
    );
    await page.getByLabel("Amount · JPY", { exact: true }).fill("300");
    assert.equal(
      await page
        .getByRole("dialog")
        .getByLabel("Account", { exact: true })
        .locator("option")
        .count(),
      1,
    );
    await preview();
    await save();
    assert.equal((await state(yen.id)).balance_minor, 9700);
    passed.push(
      "Refunds and travel: refund classification, whole-yen precision and separate currencies survive editing",
    );
    const csv =
      "Date,Amount,Description,Category,Reference\n2024-02-03,-3.21,Bank coffee,Food,BANK-A\n";
    const input = {
      account_id: bank.id,
      filename: "bank.csv",
      csv,
      delimiter: "comma",
      mapping: {
        date: 0,
        amount: 1,
        note: 2,
        category: 3,
        reference: 4,
        amount_mode: "signed",
        date_format: "iso",
        number_style: "dot",
        sign_convention: "income_positive",
      },
      starting_balance: { mode: "existing", amount_minor: 0, date: "" },
    };
    const imported = await api("POST", "/v1/imports/preview", input),
      sourceImport = await api("POST", `/v1/imports/${imported.id}/commit`, {
        rows: [2],
        duplicate_approvals: [],
      });
    await openEdit(sourceImport.transaction_ids[0]);
    assert.equal(
      await page
        .getByRole("checkbox", { name: "Use a calendar date without a time" })
        .isChecked(),
      true,
    );
    await page
      .getByLabel("Note · optional", { exact: true })
      .fill("Bank row checked");
    await preview();
    await save();
    const updated = (
      await api(
        "GET",
        `/v1/activity?transaction_id=${sourceImport.transaction_ids[0]}`,
      )
    ).entries[0];
    const replacement = (
      await api("GET", `/v1/activity?transaction_id=${updated.corrected_by}`)
    ).entries[0];
    assert.equal(replacement.import_id, imported.id);
    assert.equal(replacement.import_date_only, true);
    assert.equal(replacement.occurred_at, "2024-02-03T00:00:00Z");
    const duplicate = await api("POST", "/v1/imports/preview", input);
    assert.equal(duplicate.preview.ready_count, 0);
    assert.equal(duplicate.preview.duplicate_count, 1);
    passed.push(
      "Spreadsheet switching: correcting an imported row keeps its bank date, receipt provenance and reimport duplicate detection",
    );
    const currentBank = await state(bank.id);
    await api("POST", `/v1/accounts/${bank.id}/balance-checks`, {
      actual_minor: currentBank.balance_minor,
      note: "Posted bank balance",
    });
    await page.evaluate(
      (id) =>
        localStorage.setItem(
          `ledgerx.appearance.${id}`,
          JSON.stringify({ icon: "card", color: "sky" }),
        ),
      bank.id,
    );
    await page.goto(ui + "/workspace");
    await page
      .getByRole("button", { name: "Download workspace backup", exact: true })
      .waitFor();
    await audit("backup-tools-desktop");
    await page.setViewportSize({ width: 320, height: 900 });
    await audit("backup-tools-320");
    await page.setViewportSize({ width: 1440, height: 1100 });
    const pendingDownload = page.waitForEvent("download");
    await page
      .getByRole("button", { name: "Download workspace backup", exact: true })
      .click();
    const download = await pendingDownload;
    const backupPath = `${out}/workspace.ledgerx.json`;
    await download.saveAs(backupPath);
    const backup = JSON.parse(fs.readFileSync(backupPath, "utf8"));
    assert.equal(backup.format, "ledgerx.workspace");
    assert.equal(backup.version, 1);
    assert.equal(backup.sha256.length, 64);
    assert.equal(backup.data.corrections.length, 6);
    assert.equal(backup.data.origins.length, 1);
    for (const field of [
      "password_hash",
      "refresh_token",
      "request_key",
      "request_hash",
      "file_path",
      "user_id",
    ])
      assert.equal(
        JSON.stringify(backup).includes(`"${field}"`),
        false,
        `private server field ${field}`,
      );
    await page
      .getByLabel("Workspace backup file", { exact: true })
      .setInputFiles(backupPath);
    await page
      .getByRole("button", { name: "Preview restore", exact: true })
      .click();
    await page
      .getByRole("heading", { name: "Check your accounts and balances." })
      .waitFor();
    assert.equal(
      await page
        .getByRole("button", { name: "Restore workspace", exact: true })
        .isDisabled(),
      true,
    );
    await audit("restore-nonempty-desktop");
    // Exercise the real sign-out/signup path into an empty recovery workspace.
    await page
      .getByRole("button", {
        name: "Sign out and create a new workspace",
        exact: true,
      })
      .click();
    await page.waitForURL(/\/register\?next=restore/);
    const registered = page.waitForResponse(
      (r) =>
        new URL(r.url()).pathname === "/auth/register" &&
        r.request().method() === "POST",
    );
    await page
      .getByLabel("Email address", { exact: true })
      .fill(`restored-${Date.now()}@example.com`);
    await page
      .getByLabel("Password", { exact: true })
      .fill(crypto.randomBytes(18).toString("hex"));
    await page
      .getByRole("button", { name: "Create account", exact: true })
      .click();
    const signup = await registered;
    assert.equal(signup.status(), 201);
    const restoredTokens = await signup.json();
    headers = { Authorization: `Bearer ${restoredTokens.access_token}` };
    await page.waitForURL(ui + "/workspace");
    await page
      .getByLabel("Workspace backup file", { exact: true })
      .setInputFiles(backupPath);
    await page
      .getByRole("button", { name: "Preview restore", exact: true })
      .click();
    await page
      .getByRole("heading", { name: "Check your accounts and balances." })
      .waitFor();
    await page.setViewportSize({ width: 320, height: 900 });
    await audit("restore-review-320");
    await page.setViewportSize({ width: 1440, height: 1100 });
    await page
      .getByRole("checkbox", {
        name: "I reviewed the accounts and balances in this backup.",
      })
      .check();
    await page.route(
      "**/v1/workspace/restores/*/commit",
      async (route) => {
        await route.fetch();
        await route.abort("failed");
      },
      { times: 1 },
    );
    await page
      .getByRole("button", { name: "Restore workspace", exact: true })
      .click();
    await page
      .getByRole("heading", { name: "Restore complete" })
      .waitFor();
    await audit("restore-complete-desktop");
    await page.setViewportSize({ width: 320, height: 900 });
    await audit("restore-complete-320");
    await page.setViewportSize({ width: 1440, height: 1100 });
    const restoreID = Number(
      new URL(page.url()).searchParams.get("restore_id"),
    );
    await page.reload();
    await page
      .getByRole("heading", { name: "Restore complete" })
      .waitFor();
    const restoredView = await api(
      "GET",
      `/v1/workspace/restores/${restoreID}`,
    );
    assert.equal(
      restoredView.receipt.summary.transaction_count,
      backup.data.transactions.length,
    );
    assert.equal(restoredView.receipt.summary.correction_count, 6);
    const restoredBank = restoredView.receipt.accounts.find(
      (a) => a.name === "Everyday bank",
    );
    assert.notEqual(restoredBank.id, bank.id);
    assert.deepEqual(
      await page.evaluate(
        (id) => JSON.parse(localStorage.getItem(`ledgerx.appearance.${id}`)),
        restoredBank.id,
      ),
      { icon: "card", color: "sky" },
    );
    assert.equal(
      (await state(restoredBank.id)).balance_minor,
      currentBank.balance_minor,
    );
    const repeatedImport = await api("POST", "/v1/imports/preview", {
      ...input,
      account_id: restoredBank.id,
    });
    assert.equal(repeatedImport.preview.ready_count, 0);
    assert.equal(repeatedImport.preview.duplicate_count, 1);
    const checks = await api(
      "GET",
      `/v1/accounts/${restoredBank.id}/balance-checks`,
    );
    assert.equal(checks.checks[0].difference_minor, 0);
    assert.equal(checks.checks[0].ledger_revision, checks.current_revision);
    const denied = await page.request.get(
      base + `/v1/workspace/restores/${restoreID}`,
      { headers: sourceHeaders },
    );
    assert.equal(denied.status(), 404);
    const report = await api("POST", "/v1/exports?type=all");
    let reportState;
    for (let i = 0; i < 40; i++) {
      reportState = await api("GET", `/v1/exports/${report.id}/status`);
      if (reportState.status === "done" || reportState.status === "error")
        break;
      await page.waitForTimeout(500);
    }
    assert.equal(reportState.status, "done");
    const reportResponse = await page.request.get(
      base + `/v1/exports/${report.id}/download`,
      { headers },
    );
    assert.equal(reportResponse.status(), 200);
    const reportPath = `${out}/restored-report.csv`;
    fs.writeFileSync(reportPath, await reportResponse.body());
    const rows = JSON.parse(
      execFileSync(
        "python3",
        [
          "-c",
          "import csv,json,sys\nwith open(sys.argv[1],newline='') as stream:\n print(json.dumps(list(csv.DictReader(stream))))",
          reportPath,
        ],
        { encoding: "utf8" },
      ),
    );
    assert(rows.some((r) => r.correction_of));
    assert(rows.some((r) => r.corrected_by));
    assert(
      rows.some(
        (r) =>
          r.import_id &&
          r.date_only === "true" &&
          r.note === "Bank row checked",
      ),
    );
    await page
      .getByRole("button", { name: "Back to backup tools", exact: true })
      .click();
    backup.data.transactions[0].note = "Tampered note";
    await page
      .getByLabel("Workspace backup file", { exact: true })
      .setInputFiles({
        name: "tampered.json",
        mimeType: "application/json",
        buffer: Buffer.from(JSON.stringify(backup)),
      });
    await page
      .getByRole("button", { name: "Preview restore", exact: true })
      .click();
    await page.getByRole("alert").filter({ hasText: "checksum" }).waitFor();
    assert.equal(
      (await state(restoredBank.id)).balance_minor,
      currentBank.balance_minor,
    );
    passed.push(
      "Recovery: private backup, verified preview, nonempty protection, fresh-workspace signup, atomic lost-response restore, receipt reload, account appearance and actual CSV round trip",
    );
    assert.deepEqual(errors, [], "browser errors");
    fs.writeFileSync(
      `${out}/results.json`,
      JSON.stringify(
        { synthetic: true, customerParticipants: 0, passed, audits },
        null,
        2,
      ),
    );
    console.log(
      JSON.stringify(
        {
          synthetic: true,
          passed,
          axeStates: audits.length,
          violations: 0,
          output: out,
        },
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
