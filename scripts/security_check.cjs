// Account lifecycle regression using synthetic records only. Never records
// recovery codes, passwords, or tokens in the shareable result artifacts.
const { chromium } = require(
  process.env.PLAYWRIGHT_MODULE_PATH || "./browser/node_modules/playwright",
);
const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const fs = require("node:fs");

(async () => {
  const out = process.env.LEDGERX_QA_OUTPUT || "/tmp/ledgerx-security-check";
  fs.mkdirSync(out, { recursive: true, mode: 0o700 });
  const ui = (process.env.LEDGERX_UI_URL || "http://127.0.0.1:8088").replace(
    /\/$/,
    "",
  );
  const api = (process.env.LEDGERX_API_URL || ui).replace(/\/$/, "");
  const browser = await chromium.launch({
    headless: true,
    ...(process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH
      ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH }
      : {}),
    ...(process.env.PLAYWRIGHT_NO_SANDBOX === "1"
      ? { args: ["--no-sandbox"] }
      : {}),
  });
  const page = await browser.newPage({
    viewport: { width: 390, height: 844 },
    timezoneId: "Asia/Dhaka",
  });
  const errors = [],
    passed = [],
    audits = [];
  page.on("pageerror", (error) => errors.push(error.message));
  async function call(method, path, data, token = "") {
    for (let attempt = 0; attempt < 8; attempt++) {
      const response = await page.request.fetch(api + path, {
        method,
        data,
        headers: token ? { Authorization: "Bearer " + token } : {},
      });
      if (response.status() !== 429) return response;
      await page.waitForTimeout(1100);
    }
    throw Error("Rate limit did not clear");
  }
  async function audit(name) {
    await page.addScriptTag({
      path:
        process.env.AXE_SCRIPT_PATH ||
        require.resolve("./browser/node_modules/axe-core/axe.min.js"),
    });
    const violations = await page.evaluate(async () =>
      (
        await window.axe.run(document, {
          runOnly: {
            type: "tag",
            values: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"],
          },
          resultTypes: ["violations"],
        })
      ).violations.map((v) => ({
        id: v.id,
        impact: v.impact,
        targets: v.nodes.map((n) => n.target),
      })),
    );
    audits.push({ name, violations });
    assert.equal(violations.length, 0, `${name}: accessibility violations`);
    assert.ok(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
      name + " horizontal overflow",
    );
  }
  try {
    const email = `security-${Date.now()}-${crypto.randomBytes(4).toString("hex")}@example.com`;
    const original = crypto.randomBytes(18).toString("hex"),
      changed = crypto.randomBytes(18).toString("hex"),
      recovered = crypto.randomBytes(18).toString("hex");
    const registered = await call("POST", "/auth/register", {
      email,
      password: original,
    });
    assert.equal(registered.status(), 201);
    const tokens = await registered.json();
    await page.goto(ui + "/login");
    await page.evaluate((tokens) => {
      localStorage.setItem("access_token", tokens.access_token);
      localStorage.setItem("refresh_token", tokens.refresh_token);
    }, tokens);
    const accountResponse = await call(
      "POST",
      "/v1/accounts",
      { name: "Synthetic deletion account", currency: "USD" },
      tokens.access_token,
    );
    assert.equal(accountResponse.status(), 201);
    const account = await accountResponse.json();
    const transaction = await call(
      "POST",
      "/v1/transactions",
      {
        account_id: account.id,
        amount_minor: -250,
        currency: "USD",
        occurred_at: "2026-01-02T12:00:00Z",
        note: "Synthetic security test",
      },
      tokens.access_token,
    );
    assert.equal(transaction.status(), 201);
    await page.goto(ui + "/security");
    await page.getByRole("heading", { name: "Save a recovery code" }).waitFor();
    await audit("account-security-mobile");
    await page
      .getByLabel("Current password for recovery code")
      .fill("wrong-password");
    await page
      .getByRole("button", { name: "Generate recovery code", exact: true })
      .click();
    await page
      .getByRole("alert")
      .filter({ hasText: "current password does not match" })
      .waitFor();
    assert.match(page.url(), /\/security$/);
    await page.getByLabel("Current password for recovery code").fill(original);
    await page
      .getByRole("button", { name: "Generate recovery code", exact: true })
      .click();
    const field = page.getByLabel("Recovery code", { exact: true });
    await field.waitFor();
    const recoveryCode = await field.inputValue();
    assert.match(recoveryCode, /^lxr_[a-f0-9]{64}$/);
    const downloadEvent = page.waitForEvent("download");
    await page.getByRole("button", { name: "Download recovery code" }).click();
    const downloaded = await downloadEvent;
    assert.ok(
      fs.readFileSync(await downloaded.path(), "utf8").includes(recoveryCode),
    );
    await page.getByRole("button", { name: "I saved my code" }).click();
    assert.equal(await field.count(), 0);
    const state = await call(
      "GET",
      "/v1/security",
      undefined,
      tokens.access_token,
    );
    const stateText = await state.text();
    assert.ok(!stateText.includes(recoveryCode) && !stateText.includes("hash"));
    passed.push(
      "wrong-password refusal preserves the session; a recovery code is shown once and downloads privately",
    );
    await page.getByLabel("Current password", { exact: true }).fill(original);
    await page.getByLabel("New password", { exact: true }).fill(changed);
    await page
      .getByLabel("Confirm new password", { exact: true })
      .fill(changed);
    await page
      .getByRole("button", { name: "Change password and log out" })
      .click();
    await page.waitForURL(/\/login\?password_changed=1$/);
    assert.equal(
      (
        await call("GET", "/v1/users/me", undefined, tokens.access_token)
      ).status(),
      401,
    );
    assert.equal(
      (
        await call("POST", "/auth/refresh", {
          refresh_token: tokens.refresh_token,
        })
      ).status(),
      401,
    );
    await audit("password-change-login");
    passed.push("password change ends access and refresh sessions");
    await page.getByRole("link", { name: "Forgot your password?" }).click();
    await page.getByRole("heading", { name: "Recover access" }).waitFor();
    await audit("recover-access-mobile");
    await page.getByLabel("Email address", { exact: true }).fill(email);
    await page.getByLabel("Saved recovery code").fill(recoveryCode);
    await page.getByLabel("New password", { exact: true }).fill(recovered);
    await page
      .getByLabel("Confirm new password", { exact: true })
      .fill(recovered);
    await page
      .getByRole("button", { name: "Reset password", exact: true })
      .click();
    await page.waitForURL(/\/login\?recovered=1$/);
    assert.equal(
      (
        await call("POST", "/auth/recover", {
          email,
          recovery_code: recoveryCode,
          password: recovered,
        })
      ).status(),
      401,
    );
    await page.getByLabel("Email address", { exact: true }).fill(email);
    await page.getByLabel("Password", { exact: true }).fill(recovered);
    await page.getByRole("button", { name: "Log in", exact: true }).click();
    await page.waitForURL(ui + "/");
    await page
      .getByRole("heading", { name: /overview|money|good|spending/i })
      .first()
      .waitFor();
    passed.push("saved code resets access once without email delivery");
    await page.goto(ui + "/security");
    await page.getByRole("heading", { name: "Save a recovery code" }).waitFor();
    await page
      .getByText("You have not configured a recovery code yet.", {
        exact: true,
      })
      .waitFor();
    await page.getByRole("button", { name: "Review account deletion" }).click();
    await audit("account-deletion-review-mobile");
    await page.getByRole("button", { name: "Cancel", exact: true }).click();
    await page
      .getByRole("button", { name: "Review account deletion" })
      .waitFor();
    await page.getByRole("button", { name: "Review account deletion" }).click();
    assert.ok(
      await page
        .getByRole("button", { name: "Permanently delete my account" })
        .isDisabled(),
    );
    const latestAccess = await page.evaluate(() =>
      localStorage.getItem("access_token"),
    );
    await page.getByLabel("Password to delete this account").fill(recovered);
    await page.getByLabel("Type DELETE to confirm").fill("DELETE");
    await page
      .getByRole("button", { name: "Permanently delete my account" })
      .click();
    await page.waitForURL(/\/login\?deleted=1$/);
    assert.equal(
      (await call("GET", "/v1/users/me", undefined, latestAccess)).status(),
      401,
    );
    assert.equal(
      (
        await call("POST", "/auth/login", { email, password: recovered })
      ).status(),
      401,
    );
    await audit("account-deleted-login");
    passed.push(
      "deletion supports cancellation and explicit confirmation, then removes login and revokes access",
    );
    await page.screenshot({
      path: out + "/account-deleted-mobile.png",
      fullPage: true,
    });
    assert.deepEqual(errors, []);
    fs.writeFileSync(
      out + "/results.json",
      JSON.stringify({ passed, audits, errors }, null, 2),
    );
    console.log(
      JSON.stringify({
        passed: passed.length,
        audited_states: audits.length,
        accessibility_violations: audits.reduce(
          (n, a) => n + a.violations.length,
          0,
        ),
      }),
    );
  } catch (error) {
    // Do not capture a raw recovery code if a failed step leaves it visible.
    await page
      .evaluate(() =>
        document.querySelectorAll("input, textarea").forEach((node) => {
          node.value = "";
        }),
      )
      .catch(() => {});
    await page
      .screenshot({ path: out + "/failure.png", fullPage: true })
      .catch(() => {});
    fs.writeFileSync(
      out + "/results.json",
      JSON.stringify({ passed, audits, errors, failed: true }, null, 2),
    );
    throw error;
  } finally {
    await browser.close();
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
