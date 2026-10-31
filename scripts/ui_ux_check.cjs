// Optional UI/UX regression audit. Requires a running development app, Playwright,
// Chromium, and axe-core. See docs/UI_UX_REVIEW.md for setup and configuration.
// Creates a unique synthetic user and test records in the selected API.
const { chromium } = require(
  process.env.PLAYWRIGHT_MODULE_PATH || "playwright",
);
const assert = require("node:assert/strict"),
  crypto = require("node:crypto"),
  fs = require("node:fs");
(async () => {
  const out = process.env.LEDGERX_QA_OUTPUT || "/tmp/ledgerx-ui-ux";
  fs.mkdirSync(out, { recursive: true });
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
  let page;
  const uiBase = (
    process.env.LEDGERX_UI_URL || "http://127.0.0.1:5173"
  ).replace(/\/$/, "");
  const apiBase = (
    process.env.LEDGERX_API_URL || "http://127.0.0.1:8080"
  ).replace(/\/$/, "");
  const axePath =
    process.env.AXE_SCRIPT_PATH || require.resolve("axe-core/axe.min.js");
  try {
    const ctx = await browser.newContext({
      viewport: { width: 1440, height: 1000 },
      timezoneId: "Asia/Dhaka",
      locale: "en-US",
      reducedMotion: "reduce",
    });
    page = await ctx.newPage();
    page.on("pageerror", (e) => errors.push(e.message));
    async function audit(name) {
      if (!(await page.evaluate(() => !!window.axe)))
        await page.addScriptTag({ path: axePath });
      const result = await page.evaluate(
        async () =>
          await axe.run(document, {
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
        description: v.description,
        nodes: v.nodes.map((n) => ({
          target: n.target,
          summary: n.failureSummary,
        })),
      }));
      audits.push({ name, violations });
      fs.writeFileSync(
        out + "/accessibility.json",
        JSON.stringify(audits, null, 2),
      );
    }
    async function fits(name) {
      assert.ok(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
        name + " horizontal overflow",
      );
    }
    await page.goto(uiBase + "/register");
    await page
      .getByRole("button", { name: "Create account", exact: true })
      .waitFor();
    await audit("register");
    const registration = await page.request.post(apiBase + "/auth/register", {
      data: {
        email: `ux-${Date.now()}@example.com`,
        password: crypto.randomBytes(18).toString("hex"),
      },
    });
    assert.equal(registration.status(), 201);
    const tokens = await registration.json();
    const headers = { Authorization: `Bearer ${tokens.access_token}` };
    async function api(method, path, data) {
      for (let attempt = 0; attempt < 6; attempt++) {
        const response = await page.request.fetch(apiBase + path, {
          method,
          headers,
          data,
        });
        if (response.status() === 429) {
          await page.waitForTimeout(1050);
          continue;
        }
        if (!response.ok())
          throw Error(
            `${method} ${path}: ${response.status()} ${await response.text()}`,
          );
        return await response.json();
      }
      throw Error(`Rate limit did not clear for ${method} ${path}`);
    }
    const account = await api("POST", "/v1/accounts", {
      name: "Everyday",
      currency: "USD",
      opening_balance_minor: 100000,
      opening_balance_at: "2024-01-01T00:00:00Z",
    });
    const yen = await api("POST", "/v1/accounts", {
      name: "Travel yen",
      currency: "JPY",
    });
    const food = await api("POST", "/v1/categories", { name: "Food" }),
      transport = await api("POST", "/v1/categories", { name: "Transport" });
    for (let i = 0; i < 2; i++)
      await api("POST", "/v1/transactions", {
        account_id: account.id,
        amount_minor: -400,
        currency: "USD",
        occurred_at: "2024-01-15T06:00:00Z",
        note: "Morning Coffee",
        category_id: food.id,
      });
    await api("POST", "/v1/transactions", {
      account_id: account.id,
      amount_minor: -200,
      currency: "USD",
      occurred_at: "2024-01-15T06:00:00Z",
      note: "Bus fare",
      category_id: transport.id,
    });
    for (let i = 0; i < 22; i++)
      await api("POST", "/v1/transactions", {
        account_id: account.id,
        amount_minor: -100,
        currency: "USD",
        occurred_at: new Date(Date.UTC(2024, 0, 16, 6, 0, i)).toISOString(),
        note: "Purchase " + i,
      });
    await page.evaluate((t) => {
      localStorage.setItem("access_token", t.access_token);
      localStorage.setItem("refresh_token", t.refresh_token);
    }, tokens);
    await page.goto(uiBase + "/");
    await page
      .getByRole("heading", { name: "Overview", exact: true })
      .waitFor();
    await page.getByLabel("Month", { exact: true }).fill("2024-01");
    await page
      .getByRole("heading", { name: "Spending by category", exact: true })
      .waitFor();
    await page
      .getByText("22 spending records need a category.", { exact: true })
      .waitFor();
    assert.equal(await page.locator(".weekly-review-panel").count(), 0);
    assert.equal(await page.locator(".sidebar-note").count(), 0);
    assert.equal(await page.locator(".workspace-header").count(), 0);
    await audit("overview-desktop");
    await page.screenshot({
      path: out + "/overview-desktop.png",
      fullPage: true,
    });
    for (const width of [390, 320, 640]) {
      await page.setViewportSize({ width, height: 844 });
      await fits("overview " + width);
      await audit("overview-" + width);
      await page.screenshot({
        path: out + `/overview-${width}.png`,
        fullPage: true,
      });
    }
    passed.push(
      "compact overview, conditional review prompt, no decorative sidebar or duplicate header; 320/390/640px reflow",
    );
    await page.setViewportSize({ width: 390, height: 844 });
    await page.getByRole("link", { name: "Activity", exact: true }).click();
    await page.locator(".activity-row").first().waitFor();
    assert.equal(
      await page
        .getByRole("button", { name: "Filters", exact: true })
        .getAttribute("aria-expanded"),
      "false",
    );
    assert.equal(
      await page.getByLabel("Type", { exact: true }).isVisible(),
      false,
    );
    const row = await page.locator(".activity-row").first().boundingBox();
    assert.ok(row.y < 650, `first activity row too far down: ${row.y}`);
    await audit("activity-mobile-collapsed");
    await page.screenshot({ path: out + "/activity-390.png", fullPage: true });
    const toggle = page.getByRole("button", { name: "Filters", exact: true });
    await toggle.focus();
    await page.keyboard.press("Enter");
    assert.equal(await toggle.getAttribute("aria-expanded"), "true");
    await page.getByLabel("Type", { exact: true }).selectOption("expense");
    await page.locator(".activity-row").first().waitFor();
    await fits("activity filters");
    await audit("activity-mobile-expanded");
    await page
      .getByRole("button", { name: "Clear all filters", exact: true })
      .click();
    await page.waitForURL("**/activity");
    assert.equal(
      await page
        .getByRole("button", { name: "Filters", exact: true })
        .getAttribute("aria-expanded"),
      "false",
    );
    for (const width of [320, 640]) {
      await page.setViewportSize({ width, height: 844 });
      await fits("activity " + width);
      await audit("activity-" + width);
    }
    passed.push(
      "Activity filters collapsed on arrival, keyboard toggle, clear resets controls, first record visible within 650px on phone",
    );
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.getByRole("link", { name: "Overview", exact: true }).click();
    await page
      .getByRole("button", { name: "Add transaction", exact: true })
      .click();
    let writes = 0;
    page.on("request", (r) => {
      if (r.method() === "POST" && r.url().endsWith("/v1/transactions"))
        writes++;
    });
    await page.getByLabel("Amount", { exact: true }).fill("4.50");
    await page.getByLabel("Note optional").fill("Morning Coffee");
    assert.equal(
      await page.getByLabel("Date & time", { exact: true }).isVisible(),
      false,
    );
    await page
      .getByRole("button", { name: "Suggest category", exact: true })
      .click();
    await page.getByRole("button", { name: "Use Food", exact: true }).waitFor();
    assert.equal(
      await page.getByLabel("Category optional", { exact: true }).inputValue(),
      "",
    );
    assert.equal(writes, 0);
    await page
      .getByText("Used for 2 of your last 2 matching entries.", { exact: true })
      .waitFor();
    await audit("category-hint-desktop");
    for (const width of [390, 320]) {
      await page.setViewportSize({ width, height: 844 });
      await fits("entry dialog " + width);
      assert.ok(
        await page
          .getByRole("dialog")
          .evaluate((el) => el.scrollWidth <= el.clientWidth),
        "dialog overflow",
      );
      await audit("entry-hint-" + width);
      await page.screenshot({
        path: out + `/entry-${width}.png`,
        fullPage: true,
      });
    }
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.getByRole("button", { name: "Use Food", exact: true }).click();
    assert.equal(
      await page.getByLabel("Category optional", { exact: true }).inputValue(),
      String(food.id),
    );
    await page.locator(".entry-date-details > summary").click();
    await page
      .getByLabel("Date & time", { exact: true })
      .fill("2024-01-15T12:00");
    for (let i = 0; i < 24; i++) {
      await page.keyboard.press("Tab");
      assert.ok(
        await page
          .getByRole("dialog")
          .evaluate((el) => el.contains(document.activeElement)),
        "focus escaped dialog",
      );
    }
    await page
      .getByRole("button", { name: "Record expense", exact: true })
      .click();
    await page.getByRole("dialog").waitFor({ state: "detached" });
    assert.equal(writes, 1);
    await page
      .getByRole("button", { name: "Dismiss confirmation", exact: true })
      .click();
    assert.equal(
      await page
        .getByRole("button", { name: "Dismiss confirmation", exact: true })
        .count(),
      0,
    );
    passed.push(
      "free personal hint explains evidence, requires explicit acceptance, preserves date editing and keyboard focus, records only on submit, dismissible confirmation",
    );
    await page
      .getByRole("button", { name: "Add transaction", exact: true })
      .click();
    await page.getByLabel("Amount", { exact: true }).fill("1.00");
    await page.getByLabel("Note optional").fill("Morning Coffee");
    let release, started;
    const held = new Promise((r) => (release = r)),
      seen = new Promise((r) => (started = r));
    await page.route("**/v1/categories/suggest", async (route) => {
      const response = await route.fetch();
      started();
      await held;
      await route.fulfill({ response });
    });
    await page
      .getByRole("button", { name: "Suggest category", exact: true })
      .click();
    await seen;
    await page.getByLabel("Note optional").fill("Bus fare");
    release();
    await page
      .getByRole("button", { name: "Suggest category", exact: true })
      .waitFor();
    assert.equal(
      await page.getByRole("button", { name: "Use Food", exact: true }).count(),
      0,
    );
    assert.equal(
      await page.getByLabel("Category optional", { exact: true }).inputValue(),
      "",
    );
    await page.unroute("**/v1/categories/suggest");
    await page
      .getByRole("button", { name: "Suggest category", exact: true })
      .click();
    await page
      .getByRole("button", { name: "Use Transport", exact: true })
      .waitFor();
    await page.getByRole("radio", { name: "Income", exact: true }).check();
    assert.equal(
      await page
        .getByRole("button", { name: "Use Transport", exact: true })
        .count(),
      0,
    );
    await page.getByRole("radio", { name: "Expense", exact: true }).check();
    await page
      .getByLabel("Account", { exact: true })
      .selectOption(String(yen.id));
    assert.equal(
      await page
        .getByRole("button", { name: "Use Transport", exact: true })
        .count(),
      0,
    );
    assert.equal(
      await page.getByLabel("Amount", { exact: true }).inputValue(),
      "",
    );
    await page.keyboard.press("Escape");
    passed.push(
      "late hints are hidden after note/type/currency changes and never overwrite a category",
    );
    await page
      .getByRole("button", { name: "Add transaction", exact: true })
      .click();
    await page
      .getByLabel("Account", { exact: true })
      .selectOption(String(account.id));
    await page.getByLabel("Amount", { exact: true }).fill("1.00");
    await page.getByLabel("Note optional").fill("Food supplies");
    await page
      .getByRole("button", { name: "Suggest category", exact: true })
      .click();
    await page.getByRole("button", { name: "Use Food", exact: true }).waitFor();
    await page
      .getByText("This category name matches a word in your note.", {
        exact: true,
      })
      .waitFor();
    await page.getByLabel("Note optional").fill("Unknown note");
    await page.route("**/v1/categories/suggest", (r) =>
      r.fulfill({ status: 503, body: "temporarily unavailable" }),
    );
    await page
      .getByRole("button", { name: "Suggest category", exact: true })
      .click();
    await page
      .getByText(
        "Couldn't get a suggestion. You can still choose a category.",
        { exact: true },
      )
      .waitFor();
    assert.equal(
      await page
        .getByRole("button", { name: "Record expense", exact: true })
        .isEnabled(),
      true,
    );
    await page
      .getByLabel("Category optional", { exact: true })
      .selectOption(String(transport.id));
    await page.unroute("**/v1/categories/suggest");
    await page.keyboard.press("Escape");
    passed.push("free keyword fallback and nonblocking hint-service failure");
    await page.setViewportSize({ width: 320, height: 844 });
    await page.getByRole("link", { name: "Reports", exact: true }).click();
    await page
      .getByRole("button", { name: "Generate CSV", exact: true })
      .waitFor();
    await fits("reports mobile");
    await audit("reports-mobile");
    assert.deepEqual(errors, []);
    const failures = audits.flatMap((a) =>
      a.violations.map((v) => ({ page: a.name, ...v })),
    );
    console.log(
      JSON.stringify({
        passed,
        browserErrors: errors,
        axeAudits: audits.length,
        axeViolations: failures.map((v) => ({
          page: v.page,
          id: v.id,
          impact: v.impact,
          nodes: v.nodes.map((n) => n.target),
        })),
        artifacts: out,
      }),
    );
    if (failures.length) process.exitCode = 2;
  } catch (e) {
    if (page) {
      await page
        .screenshot({ path: out + "/failure.png", fullPage: true })
        .catch(() => {});
      console.log("Failure URL:", page.url());
    }
    throw e;
  } finally {
    await browser.close();
  }
})().catch((e) => {
  console.error(e);
  process.exit(1);
});
