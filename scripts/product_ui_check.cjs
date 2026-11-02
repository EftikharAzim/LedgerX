// Product navigation and exact-money layout regressions, using synthetic data.
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
  const out = process.env.LEDGERX_QA_OUTPUT || "/tmp/ledgerx-product-ui";
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
  async function api(method, path, data) {
    for (let attempt = 0; attempt < 8; attempt++) {
      const response = await context.request.fetch(base + path, {
        method,
        data,
        headers: tokens
          ? { Authorization: `Bearer ${tokens.access_token}` }
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
    assert(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 1,
      ),
      name + " horizontal overflow",
    );
    audits.push({ name, violations });
    await page.screenshot({ path: `${out}/${name}.png`, fullPage: true });
  }
  async function fittedAmounts() {
    const values = await page.locator(".fit-money").evaluateAll((items) =>
      items.map((container) => {
        const value = container.firstElementChild,
          rect = value.getBoundingClientRect(),
          parent = container.getBoundingClientRect();
        const range = document.createRange();
        range.selectNodeContents(value);
        return {
          text: value.textContent,
          width: rect.width,
          available: parent.width,
          lines: range.getClientRects().length,
          size: parseFloat(getComputedStyle(value).fontSize),
        };
      }),
    );
    assert(values.length > 0, "No money values rendered");
    for (const value of values) {
      assert(value.width <= value.available + 1, JSON.stringify(value));
      assert.equal(value.lines, 1, JSON.stringify(value));
      assert(
        value.size >= 16,
        "Money became too small: " + JSON.stringify(value),
      );
    }
  }
  try {
    await page.goto(ui + "/login");
    await page.getByRole("heading", { name: "Log in", exact: true }).waitFor();
    assert.equal(await page.locator(".auth-story,.auth-ledger-art").count(), 0);
    await audit("login-desktop");
    await page.goto(ui + "/register");
    await page
      .getByRole("heading", { name: "Create account", exact: true })
      .waitFor();
    await page.setViewportSize({ width: 320, height: 900 });
    await audit("register-320");
    tokens = await api("POST", "/auth/register", {
      email: `product-${crypto.randomUUID()}@example.com`,
      password,
    });
    await page.evaluate((t) => {
      localStorage.setItem("access_token", t.access_token);
      localStorage.setItem("refresh_token", t.refresh_token);
    }, tokens);
    const account = await api("POST", "/v1/accounts", {
      name: "Everyday",
      currency: "BDT",
      opening_balance_minor: 850000,
      opening_balance_at: "2026-01-01T00:00:00Z",
    });
    for (const entry of [
      { amount: 7000000, note: "Salary" },
      { amount: -248000, note: "Groceries" },
    ]) {
      await api("POST", "/v1/transactions", {
        account_id: account.id,
        currency: "BDT",
        amount_minor: entry.amount,
        occurred_at: new Date(Date.now() - 60000).toISOString(),
        note: entry.note,
      });
    }
    for (const width of [320, 390, 640, 1440]) {
      await page.setViewportSize({ width, height: 1000 });
      await page.goto(ui + "/");
      await page.locator(".current-balance .fit-money").waitFor();
      await page.getByText("BDT 70,000.00", { exact: true }).first().waitFor();
      assert.equal(
        await page.getByLabel("Currency", { exact: true }).count(),
        0,
        "Single-currency chooser should be absent",
      );
      await fittedAmounts();
      const monthField = await page
        .getByLabel("Month", { exact: true })
        .boundingBox();
      assert(
        monthField.width >= 190,
        "Month text lacks room beside its calendar control",
      );
      if (width <= 390) {
        const balance = await page.locator(".current-balance").boundingBox();
        assert(balance.y < 300, "Balance is below unnecessary controls");
      }
      await audit(`overview-${width}`);
      const nav = page.getByRole("navigation", { name: "Main navigation" });
      assert.equal(await nav.getByRole("link").count(), 5);
      const bounds = await nav.getByRole("link").evaluateAll((links) =>
        links.map((a) => {
          const r = a.getBoundingClientRect();
          return { left: r.left, right: r.right, width: r.width };
        }),
      );
      assert(
        bounds.every((r) => r.left >= 0 && r.right <= width && r.width > 0),
      );
      await nav.getByRole("link", { name: "More", exact: true }).click();
      await page.getByRole("heading", { name: "More", exact: true }).waitFor();
      const links = page.locator(".more-groups a");
      assert.equal(await links.count(), 9);
      for (const link of await links.all()) assert(await link.isVisible());
      await audit(`more-${width}`);
    }
    await page.setViewportSize({ width: 390, height: 1000 });
    const destinations = [
      ["Planned activity", "Planned activity"],
      ["Recurring bills & income", "Recurring bills & income"],
      ["Accounts", "Accounts"],
      ["Categories", "Categories"],
      ["Import CSV", "Import CSV"],
      ["Export CSV", "Export CSV"],
      ["Backup & restore", "Backup & restore"],
      ["Account & security", "Account & security"],
      ["Getting started", "Getting started"],
    ];
    for (const [label, title] of destinations) {
      await page.goto(ui + "/more");
      const link = page
        .locator(".more-groups")
        .getByRole("link", { name: label, exact: true });
      await link.focus();
      await page.keyboard.press("Enter");
      await page.getByRole("heading", { name: title, exact: true }).waitFor();
    }
    passed.push(
      "All nine secondary tools remain reachable by keyboard through More on phones",
    );
    await page.goto(ui + "/exports");
    await page
      .getByRole("heading", { name: "Export CSV", exact: true })
      .waitFor();
    assert.equal(
      await page.locator(".export-preview,.export-paper,.export-empty").count(),
      0,
    );
    await audit("export-390");
    await page.getByText("What’s included", { exact: true }).click();
    await audit("export-details-390");
    await page
      .getByRole("button", { name: "Generate CSV", exact: true })
      .click();
    const downloadButton = page.getByRole("button", {
      name: "Download CSV",
      exact: true,
    });
    await downloadButton.waitFor();
    await audit("export-ready-390");
    const downloaded = page.waitForEvent("download");
    await downloadButton.click();
    const file = await downloaded;
    assert.match(file.suggestedFilename(), /^ledgerx-.*\.csv$/);
    const csv = fs.readFileSync(await file.path(), "utf8");
    assert(
      csv.includes("Salary") && csv.includes("Groceries"),
      "Export lost the selected recorded activity",
    );
    passed.push(
      "CSV generation, ready state and authenticated download remain usable without decorative panels",
    );
    for (const [currency, amount] of [
      ["USD", 123456],
      ["JPY", 123456],
    ])
      await api("POST", "/v1/accounts", {
        name: currency,
        currency,
        opening_balance_minor: amount,
        opening_balance_at: "2026-01-01T00:00:00Z",
      });
    await page.goto(ui + "/");
    for (const [currency, expected] of [
      ["USD", "$1,234.56"],
      ["JPY", "¥123,456"],
      ["BDT", "BDT 76,020.00"],
    ]) {
      await page.getByLabel("Currency", { exact: true }).selectOption(currency);
      await page.waitForFunction(
        (expected) =>
          document
            .querySelector(".current-balance .fit-money")
            ?.textContent.replace(/\s/g, " ") === expected,
        expected,
      );
      await fittedAmounts();
    }
    await audit("currencies-390");
    const savings = await api("POST", "/v1/accounts", {
      name: "Savings",
      currency: "BDT",
    });
    for (const width of [1440, 320]) {
      await page.setViewportSize({ width, height: 1000 });
      await page.goto(ui + `/accounts/${account.id}`);
      await page.locator(".hero-amount .fit-money").waitFor();
      await fittedAmounts();
      await audit(`account-details-${width}`);
      await page.getByRole("button", { name: "Transfer", exact: true }).click();
      await page
        .getByRole("dialog", { name: "Transfer money", exact: true })
        .waitFor();
      await page
        .getByLabel("To", { exact: true })
        .selectOption(String(savings.id));
      await page.getByLabel("Amount", { exact: true }).fill("1250.75");
      await audit(`transfer-entry-${width}`);
      await page
        .getByRole("button", { name: "Review transfer", exact: true })
        .click();
      await page
        .getByRole("dialog", { name: "Review transfer", exact: true })
        .waitFor();
      await fittedAmounts();
      await audit(`transfer-review-${width}`);
      if (width === 320) {
        await page
          .getByRole("button", { name: "Confirm transfer", exact: true })
          .click();
        await page.getByRole("dialog").waitFor({ state: "hidden" });
        const source = await api("GET", `/v1/accounts/${account.id}/balance`);
        const target = await api("GET", `/v1/accounts/${savings.id}/balance`);
        assert.equal(source.balance_minor, 7476925);
        assert.equal(target.balance_minor, 125075);
        assert.equal(source.balance_minor + target.balance_minor, 7602000);
      } else {
        await page.keyboard.press("Escape");
      }
      await page.goto(ui + "/categories");
      await page
        .getByRole("button", { name: "New category", exact: true })
        .click();
      await page
        .getByRole("dialog", { name: "New category", exact: true })
        .waitFor();
      await page.getByLabel("Category name", { exact: true }).fill("Groceries");
      await audit(`new-category-${width}`);
      if (width === 320) {
        await page
          .getByRole("button", { name: "Create category", exact: true })
          .click();
        await page.getByRole("dialog").waitFor({ state: "hidden" });
      } else {
        await page.keyboard.press("Escape");
      }
    }
    await page.goto(ui + "/accounts");
    await page
      .getByRole("button", { name: "New account", exact: true })
      .click();
    const appearance = page.getByText("Icon & color", { exact: true });
    assert.equal(
      await page.getByLabel("lavender", { exact: true }).isVisible(),
      false,
    );
    await audit("new-account-320");
    await appearance.click();
    await page.getByLabel("lavender", { exact: true }).check();
    await page.getByLabel("Account name", { exact: true }).fill("Mobile cash");
    await page.getByLabel("Currency", { exact: true }).selectOption("BDT");
    await audit("account-appearance-320");
    await page
      .getByRole("button", { name: "Create account", exact: true })
      .click();
    await page.getByRole("dialog").waitFor({ state: "hidden" });
    await page.reload();
    await page
      .locator(".account-card.accent-lavender")
      .filter({ hasText: "Mobile cash" })
      .waitFor();

    const entry = await api("POST", "/v1/transactions", {
      account_id: account.id,
      currency: "BDT",
      amount_minor: -123,
      occurred_at: new Date(Date.now() - 60000).toISOString(),
      note: "Groceries market",
    });
    let suggestion;
    for (let attempt = 0; attempt < 30; attempt++) {
      const history = await api(
        "GET",
        `/v1/accounts/${account.id}/transactions?limit=100`,
      );
      suggestion = history.entries.find(
        (item) => item.transaction_id === entry.id,
      );
      if (suggestion?.suggested_category_id) break;
      await new Promise((resolve) => setTimeout(resolve, 1000));
    }
    assert(
      suggestion?.suggested_category_id,
      "Keyword suggestion was not prepared",
    );
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.goto(ui + `/accounts/${account.id}`);
    await page
      .locator(".activity-row")
      .filter({ hasText: "Groceries market" })
      .waitFor();
    const suggestedRow = page
      .locator(".activity-row")
      .filter({ hasText: "Groceries market" });
    assert.match(await suggestedRow.innerText(), /Suggested: Groceries/);
    await audit("category-suggestion-desktop");
    await suggestedRow.click();
    await page
      .getByText("Suggested category · not applied", { exact: true })
      .waitFor();
    await page.getByRole("button", { name: "Accept", exact: true }).click();
    await page.getByText("Category saved", { exact: true }).waitFor();
    await audit("category-confirmed-desktop");
    await page.keyboard.press("Escape");
    passed.push(
      "Plain transfer and category dialogs retain keyboard review and saving; optional appearance persists; suggestions stay explicit until accepted",
    );

    const maximumAccount = await api("POST", "/v1/accounts", {
      name: "Exact limit",
      currency: "BDT",
      opening_balance_minor: Number.MAX_SAFE_INTEGER,
      opening_balance_at: "2026-01-01T00:00:00Z",
    });
    await page.goto(ui + "/");
    await page.getByText("Out of range", { exact: true }).waitFor();
    const exact = page.locator(".account-card").filter({
      has: page.getByRole("heading", { name: "Exact limit", exact: true }),
    });
    await exact.locator(".fit-money").waitFor();
    assert.equal(
      (await exact.locator(".fit-money").innerText()).replace(/\s/g, " "),
      "BDT 90,071,992,547,409.91",
    );
    await fittedAmounts();
    await audit("exact-limit-390");
    await page.setViewportSize({ width: 320, height: 1000 });
    await fittedAmounts();
    await audit("exact-limit-320");
    passed.push(
      "BDT/USD/JPY and the exact supported limit retain all digits; an unsupported combined balance is reported instead of rounded",
    );
    for (const width of [320, 1440]) {
      await page.setViewportSize({ width, height: 1000 });
      await page.goto(ui + `/accounts/${maximumAccount.id}`);
      await page.locator(".hero-amount .fit-money").waitFor();
      await fittedAmounts();
      const overlap = await page
        .locator(".activity-row")
        .first()
        .evaluate((row) => {
          const name = row
            .querySelector(".activity-description")
            .getBoundingClientRect();
          const money = row
            .querySelector(".activity-amount")
            .getBoundingClientRect();
          return (
            name.left < money.right &&
            name.right > money.left &&
            name.top < money.bottom &&
            name.bottom > money.top
          );
        });
      assert.equal(
        overlap,
        false,
        "Amount overlaps the transaction description",
      );
      await audit(`maximum-account-${width}`);
      await page.locator(".activity-row").first().click();
      await page
        .getByRole("dialog", { name: "Transaction details", exact: true })
        .waitFor();
      assert.match(
        await page.locator(".entry-detail-amount .fit-money").innerText(),
        /^\+BDT/,
      );
      await fittedAmounts();
      await audit(`maximum-transaction-${width}`);
      await page.keyboard.press("Escape");
    }
    assert.deepEqual(
      audits.filter((state) => state.violations.length),
      [],
      "Accessibility checks failed",
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
