import assert from "node:assert/strict";
import { test } from "node:test";
import {
  reportRange,
  reportFilename,
  reportLabel,
} from "../src/lib/report-period.ts";
import {
  jobSelection,
  rememberExport,
  type ExportJob,
} from "../src/lib/export-jobs.ts";

test("monthly and yearly reports include leap day and the final calendar date", () => {
  assert.deepEqual(reportRange({ type: "monthly", month: "2024-02" }), {
    start: "2024-02-01",
    end: "2024-02-29",
  });
  assert.deepEqual(reportRange({ type: "yearly", year: "2024" }), {
    start: "2024-01-01",
    end: "2024-12-31",
  });
  assert.equal(reportRange({ type: "yearly", year: "0000" }), null);
  assert.equal(reportRange({ type: "yearly", year: "10000" }), null);
});

test("weeks normalize to Monday–Sunday across months and years", () => {
  assert.deepEqual(reportRange({ type: "weekly", week_of: "2020-12-31" }), {
    start: "2020-12-28",
    end: "2021-01-03",
  });
  assert.deepEqual(reportRange({ type: "weekly", week_of: "2026-11-01" }), {
    start: "2026-10-26",
    end: "2026-11-01",
  });
  assert.equal(reportRange({ type: "weekly", week_of: "9999-12-31" }), null);
});

test("custom periods accept a single day and reject reversed or impossible dates", () => {
  assert.deepEqual(
    reportRange({
      type: "custom",
      start_date: "2024-02-29",
      end_date: "2024-02-29",
    }),
    { start: "2024-02-29", end: "2024-02-29" },
  );
  assert.equal(
    reportRange({
      type: "custom",
      start_date: "2026-02-29",
      end_date: "2026-03-01",
    }),
    null,
  );
  assert.equal(
    reportRange({
      type: "custom",
      start_date: "2026-03-01",
      end_date: "2026-02-28",
    }),
    null,
  );
});

test("all-time reports have no artificial date boundaries", () => {
  assert.deepEqual(reportRange({ type: "all" }), { start: null, end: null });
  assert.equal(reportLabel({ type: "all" }), "All time");
  assert.equal(reportFilename({ type: "all" }), "ledgerx-all-time.csv");
});

test("filenames identify the actual selected period, including normalized weeks", () => {
  assert.equal(
    reportFilename({ type: "monthly", month: "2026-10" }),
    "ledgerx-2026-10.csv",
  );
  assert.equal(
    reportFilename({ type: "yearly", year: "2026" }),
    "ledgerx-yearly-2026.csv",
  );
  assert.equal(
    reportFilename({ type: "weekly", week_of: "2026-11-01" }),
    "ledgerx-weekly-2026-10-26_2026-11-01.csv",
  );
});

test("visit history retains new report kinds and supports earlier monthly requests", () => {
  const legacy: ExportJob = {
    id: 1,
    month: "2026-10",
    initialStatus: "pending",
    requestedAt: "2026-10-31T00:00:00Z",
  };
  const yearly: ExportJob = {
    id: 2,
    selection: { type: "yearly", year: "2024" },
    initialStatus: "pending",
    requestedAt: legacy.requestedAt,
  };
  const jobs = rememberExport([legacy], yearly);
  assert.deepEqual(jobSelection(jobs[0]), yearly.selection);
  assert.deepEqual(jobSelection(jobs[1]), {
    type: "monthly",
    month: "2026-10",
  });
});
