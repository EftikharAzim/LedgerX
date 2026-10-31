import assert from "node:assert/strict";
import { test } from "node:test";
import { loginSchema, registerSchema } from "../src/lib/auth-form.ts";
import { isValidMonth, monthLabel } from "../src/lib/month.ts";
import {
  rememberExport,
  shouldPollExport,
  exportPresentation,
  type ExportJob,
} from "../src/lib/export-jobs.ts";

test("signup trims email without changing password contents", () => {
  const value = registerSchema.parse({
    email: "  someone@example.com  ",
    password: " keep these spaces ",
  });
  assert.equal(value.email, "someone@example.com");
  assert.equal(value.password, " keep these spaces ");
});

test("signup validates character length and the server's UTF-8 password limit", () => {
  const valid = (password: string) =>
    registerSchema.safeParse({ email: "someone@example.com", password })
      .success;
  assert.equal(valid("short"), false);
  assert.equal(valid("🌿".repeat(7)), false);
  assert.equal(valid("🌿".repeat(8)), true);
  assert.equal(valid("a".repeat(72)), true);
  assert.equal(valid("a".repeat(73)), false);
  assert.equal(valid("🌿".repeat(19)), false);
});

test("login allows existing credentials without applying new signup rules", () => {
  assert.equal(
    loginSchema.safeParse({
      email: "someone@example.com",
      password: "🌿🌿🌿🌿",
    }).success,
    true,
  );
  assert.equal(
    loginSchema.safeParse({ email: "not an email", password: "password" })
      .success,
    false,
  );
  assert.equal(
    loginSchema.safeParse({ email: "someone@example.com", password: "" })
      .success,
    false,
  );
});

test("export month validation rejects empty, zero-year, and invalid month values", () => {
  for (const month of [
    "",
    "2026-0",
    "2026-00",
    "2026-13",
    "0000-01",
    "2026-01-01",
    "26-01",
  ])
    assert.equal(isValidMonth(month), false, month);
  assert.equal(isValidMonth("2026-12"), true);
  assert.equal(isValidMonth("0001-01"), true);
});

test("export month labels describe the selected UTC calendar month", () => {
  assert.equal(
    monthLabel("2026-10"),
    new Intl.DateTimeFormat(undefined, {
      month: "long",
      year: "numeric",
      timeZone: "UTC",
    }).format(new Date("2026-10-01T00:00:00Z")),
  );
  assert.equal(monthLabel(""), "Choose a month");
});

const job = (id: number, month = "2026-10"): ExportJob => ({
  id,
  month,
  requestedAt: "2026-10-31T12:00:00Z",
  initialStatus: "pending",
});

test("recent exports retain the requested month, replace duplicate IDs, and bound the visit list", () => {
  let jobs: ExportJob[] = [];
  for (let id = 1; id <= 6; id++) jobs = rememberExport(jobs, job(id));
  assert.deepEqual(
    jobs.map((value) => value.id),
    [6, 5, 4, 3, 2],
  );
  jobs = rememberExport(jobs, { ...job(3), initialStatus: "done" });
  assert.deepEqual(
    jobs.map((value) => value.id),
    [3, 6, 5, 4, 2],
  );
  assert.equal(jobs[0].initialStatus, "done");
  assert.equal(jobs[0].month, "2026-10");
});

test("export polling stops on completion, failure, network errors and unknown states", () => {
  for (const state of [undefined, "pending", "processing"])
    assert.equal(shouldPollExport(state), true);
  for (const state of ["done", "error", "unexpected"])
    assert.equal(shouldPollExport(state), false);
  assert.equal(shouldPollExport("pending", true), false);
});

test("export display keeps unknown states distinct from success", () => {
  assert.equal(exportPresentation("done").label, "Ready");
  assert.equal(exportPresentation("error").tone, "failed");
  assert.equal(exportPresentation("unexpected").label, "Status unavailable");
});
