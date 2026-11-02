// Financial account management regressions, using synthetic data.
const { chromium } = require(
  process.env.PLAYWRIGHT_MODULE_PATH || "playwright",
);
const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const fs = require("node:fs");

(async () => {
  const base = (process.env.LEDGERX_API_URL || "http://127.0.0.1:8088").replace(
    /\/$/,
    "",
  );
  const ui = (process.env.LEDGERX_UI_URL || base).replace(/\/$/, "");
  const out =
    process.env.LEDGERX_QA_OUTPUT || "/tmp/ledgerx-account-management";
  fs.mkdirSync(out, { recursive: true, mode: 0o700 });
  const browser = await chromium.launch({
    ...(process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH
      ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH }
      : {}),
    ...(process.env.PLAYWRIGHT_NO_SANDBOX === "1"
      ? { args: ["--no-sandbox"] }
      : {}),
  });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1000 },
    timezoneId: "Asia/Dhaka",
    locale: "en-US",
    reducedMotion: "reduce",
  });
  const page = await context.newPage();
  const errors = [],
    audits = [],
    passed = [];
  page.on("pageerror", (error) => errors.push(error.message));
  const password = crypto.randomBytes(20).toString("hex");
  let tokens;
  async function api(method, path, data, extra = {}) {
    for (let attempt = 0; attempt < 8; attempt++) {
      const response = await context.request.fetch(base + path, {
        method,
        data,
        headers: tokens
          ? { Authorization: `Bearer ${tokens.access_token}`, ...extra }
          : {},
      });
      if (response.status() === 429) {
        await new Promise((resolve) => setTimeout(resolve, 1000));
        continue;
      }
      assert(
        response.ok(),
        `${method} ${path}: ${response.status()} ${await response.text()}`,
      );
      return response.status() === 204 ? null : response.json();
    }
    throw new Error("Rate limit did not clear");
  }
  async function audit(name) {
    if (!(await page.evaluate(() => !!window.axe)))
      await page.addScriptTag({
        path:
          process.env.AXE_SCRIPT_PATH || require.resolve("axe-core/axe.min.js"),
      });
    const result = await page.evaluate(() =>
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
      nodes: v.nodes.map((n) => n.target),
    }));
    assert.deepEqual(violations, [], name + " accessibility");
    assert(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 1,
      ),
      name + " horizontal overflow",
    );
    audits.push({ name, violations });
    await page.screenshot({ path: `${out}/${name}.png`, fullPage: true });
  }

  try {
    tokens = await api("POST", "/auth/register", {
      email: `accounts-${crypto.randomUUID()}@example.com`,
      password,
    });
    await page.goto(ui + "/login");
    await page.evaluate((t) => {
      localStorage.setItem("access_token", t.access_token);
      localStorage.setItem("refresh_token", t.refresh_token);
    }, tokens);
    let account = await api("POST", "/v1/accounts", {
      name: "Cash",
      currency: "USD",
      opening_balance_minor: 10000,
      opening_balance_at: "2026-01-01T00:00:00Z",
    });
    const bank = await api("POST", "/v1/accounts", {
      name: "Bank",
      currency: "USD",
    });
    await api("POST", "/v1/transactions", {
      account_id: account.id,
      currency: "USD",
      amount_minor: -1200,
      occurred_at: new Date(Date.now() - 60000).toISOString(),
      note: "Synthetic groceries",
    });
    const spendingBefore = await api(
      "GET",
      "/v1/spending?type=all&currency=USD",
    );
    const openEdit = async () => {
      await page
        .getByRole("button", { name: "Edit account", exact: true })
        .click();
      await page.getByRole("dialog", { name: "Edit account" }).waitFor();
    };
    const waitClosed = () =>
      page.getByRole("dialog").waitFor({ state: "hidden" });
    await page.goto(ui + `/accounts/${account.id}`);
    await openEdit();
    assert(
      await page
        .getByLabel("Account name", { exact: true })
        .evaluate((e) => e === document.activeElement),
    );
    await page.keyboard.press("Escape");
    await waitClosed();
    assert(
      await page
        .getByRole("button", { name: "Edit account", exact: true })
        .evaluate((e) => e === document.activeElement),
    );
    await openEdit();
    await page.getByLabel("Account name", { exact: true }).fill("Bank");
    await page.getByRole("button", { name: "Save name", exact: true }).click();
    await page
      .getByRole("alert")
      .filter({ hasText: "already exists" })
      .waitFor();
    assert.equal(
      await page.getByLabel("Account name", { exact: true }).inputValue(),
      "Bank",
    );
    await audit("duplicate-name-desktop");
    await page
      .getByLabel("Account name", { exact: true })
      .fill("Everyday cash");
    await page.getByRole("button", { name: "Save name", exact: true }).click();
    await waitClosed();
    await page
      .getByRole("heading", { name: "Everyday cash", exact: true })
      .waitFor();
    account = { ...account, name: "Everyday cash" };
    passed.push(
      "Rename, duplicate feedback, keyboard opening, Escape and focus restoration",
    );

    await openEdit();
    await page
      .getByLabel("Account name", { exact: true })
      .fill("Unsaved draft");
    account = await api("PATCH", `/v1/accounts/${account.id}`, {
      name: "Changed in another tab",
      expected_name: account.name,
      expected_active: true,
    });
    await page.getByRole("button", { name: "Save name", exact: true }).click();
    await page
      .getByRole("alert")
      .filter({ hasText: "account details changed" })
      .waitFor();
    await audit("stale-edit-desktop");
    await page
      .getByRole("button", { name: "Reload details", exact: true })
      .click();
    await page.waitForFunction(
      () =>
        document.querySelector("#edit-account-name")?.value ===
        "Changed in another tab",
    );
    await page.getByRole("button", { name: "Cancel", exact: true }).click();
    await waitClosed();
    passed.push(
      "Stale edits require explicit reload and preserve the draft until requested",
    );

    const today = new Date().toISOString().slice(0, 10);
    const bill = await api(
      "POST",
      "/v1/schedules",
      {
        account_id: account.id,
        currency: "USD",
        amount_minor: -500,
        note: "Synthetic phone bill",
        cadence: "monthly",
        first_due_date: today,
      },
      { "Idempotency-Key": crypto.randomUUID() },
    );
    await page.setViewportSize({ width: 320, height: 950 });
    await openEdit();
    await audit("edit-320");
    await page
      .getByRole("button", { name: "Archive account", exact: true })
      .click();
    await audit("archive-review-320");
    await page
      .getByRole("button", { name: "Confirm archive", exact: true })
      .click();
    await page
      .getByRole("alert")
      .filter({ hasText: "active recurring schedules" })
      .waitFor();
    await audit("archive-bill-conflict-320");
    await page
      .getByRole("link", { name: "Manage recurring bills", exact: true })
      .click();
    await page
      .getByRole("heading", { name: "Recurring bills & income", exact: true })
      .waitFor();
    await api("POST", `/v1/schedules/${bill.id}/pause`, {
      revision: bill.revision,
    });
    await page.goto(ui + `/accounts/${account.id}`);
    await openEdit();
    await page
      .getByRole("button", { name: "Archive account", exact: true })
      .click();
    await page
      .getByRole("button", { name: "Confirm archive", exact: true })
      .click();
    await waitClosed();
    await page
      .locator(".detail-account-meta")
      .filter({ hasText: "Archived account" })
      .waitFor();
    assert(
      await page
        .getByRole("button", { name: "Add transaction", exact: true })
        .isDisabled(),
    );
    assert(
      await page
        .getByRole("button", { name: "Transfer", exact: true })
        .isDisabled(),
    );
    await page.getByText("Synthetic groceries", { exact: true }).waitFor();
    await audit("archived-account-320");
    assert.deepEqual(
      await api("GET", "/v1/spending?type=all&currency=USD"),
      spendingBefore,
    );
    const backup = await api("GET", "/v1/workspace/backup");
    const saved = backup.data.accounts.find((a) => a.id === account.id);
    assert.equal(saved.active, false);
    assert.equal(saved.name, account.name);
    const pending = await api("GET", "/v1/plans");
    assert(
      pending.entries.some((e) => e.account_id === account.id),
      "Due plan disappeared",
    );
    await page.goto(ui + `/accounts/${bank.id}`);
    await page.getByRole("button", { name: "Transfer", exact: true }).click();
    await page
      .getByRole("heading", {
        name: "Create another account to transfer money",
        exact: true,
      })
      .waitFor();
    await audit("archived-transfer-choice-320");
    await page.keyboard.press("Escape");
    await api("PATCH", `/v1/accounts/${bank.id}`, {
      active: false,
      expected_name: bank.name,
      expected_active: true,
    });
    await page.goto(ui + "/");
    await page
      .getByText("All accounts are archived.", { exact: false })
      .waitFor();
    assert.equal(
      await page
        .getByRole("button", { name: "Create your first account", exact: true })
        .count(),
      0,
    );
    assert.equal(
      (await page.locator(".current-balance .fit-money").innerText()).replace(
        /\s/g,
        " ",
      ),
      "$88.00",
    );
    await audit("all-accounts-archived-320");
    await api("PATCH", `/v1/accounts/${bank.id}`, {
      active: true,
      expected_name: bank.name,
      expected_active: false,
    });
    passed.push(
      "Active bills block archive; pausing allows archive; history, reports, backup and due plans remain intact",
    );

    await page.goto(ui + "/accounts");
    await page
      .getByLabel("Account status", { exact: true })
      .selectOption("archived");
    await page
      .locator(".account-card")
      .filter({ hasText: account.name })
      .waitFor();
    assert.equal(await page.locator(".account-card").count(), 1);
    await audit("archived-filter-320");
    await page
      .getByLabel("Account status", { exact: true })
      .selectOption("active");
    await page.locator(".account-card").filter({ hasText: "Bank" }).waitFor();
    assert.equal(await page.locator(".account-card").count(), 1);
    await page.goto(ui + `/accounts/${account.id}`);
    await openEdit();
    await audit("restore-320");
    await page
      .getByRole("button", { name: "Restore account", exact: true })
      .click();
    await waitClosed();
    await page
      .locator(".detail-account-meta")
      .filter({ hasText: "Active account" })
      .waitFor();
    assert(
      await page
        .getByRole("button", { name: "Add transaction", exact: true })
        .isEnabled(),
    );
    const schedules = await api("GET", "/v1/schedules");
    assert.equal(schedules.find((s) => s.id === bill.id).status, "paused");
    await audit("restored-account-320");
    passed.push(
      "Status filters and explicit restore work on phones; restoring never resumes bills",
    );
    assert.deepEqual(errors, []);
  } catch (error) {
    await page
      .screenshot({ path: out + "/failure.png", fullPage: true })
      .catch(() => {});
    fs.writeFileSync(out + "/failure.txt", String(error), { mode: 0o600 });
    throw error;
  } finally {
    if (tokens)
      await api("DELETE", "/v1/security/account", {
        current_password: password,
        confirmation: "DELETE",
      });
    await browser.close();
    fs.writeFileSync(
      out + "/results.json",
      JSON.stringify({ passed, audits, errors }, null, 2),
    );
  }
  console.log(
    JSON.stringify({
      passed,
      audited_states: audits.length,
      violations: 0,
      output: out,
    }),
  );
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
