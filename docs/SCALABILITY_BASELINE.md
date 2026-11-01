# Local scalability baseline

Recorded 2026-11-01. These are short synthetic API measurements on a managed
development machine. They are not a supported-customer count, a hosted-service
benchmark, or evidence of customer demand.

## Workload and environment

The complete Docker package ran one Go API process with four background workers,
a 10-connection database pool, local persistent export storage, PostgreSQL 16,
Redis 7, and the free keyword categorizer. The normal 5-requests/sec per-user
limiter remained enabled. No paid load-testing service was used.

The managed workspace's cgroup allowed 4 CPU cores and 32 GiB memory. Services
shared the host; there were no smaller per-container CPU/RAM limits. The load
generator ran locally over loopback on Python 3.12.14. These resources and
network conditions differ from a smaller free hosted service.

The runner created 25 synthetic users, each with 1,000 imported historical
transactions and an explicit opening balance. It then ran 1, 5, 10, and 25
simultaneous users for 15 seconds per level, with 0.4-second pauses. Requests
covered account lists, spending totals, Activity, current balances, new
expenses, and immediate retries with the same idempotency key. Each level also
generated/downloaded an all-history CSV in the background. Latency percentiles
below cover successful foreground API requests; seeding, background polling,
browser rendering, and Internet latency are excluded.

An initial engineering budget of p95 under 500 ms was selected before running.
The runner rejects unexpected errors, throttling, mismatched balances/spending,
changed retry identities, and incorrect export counts.

## Measured results

| Simultaneous synthetic users | Foreground requests | Successful requests/sec | p50 | p95 | Unexpected errors | HTTP 429 |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | 44 | 2.89 | 7.51 ms | 21.38 ms | 0 | 0 |
| 5 | 220 | 14.44 | 6.38 ms | 28.08 ms | 0 | 0 |
| 10 | 439 | 28.45 | 6.73 ms | 27.07 ms | 0 | 0 |
| 25 | 1,100 | 71.28 | 7.78 ms | 40.33 ms | 0 | 0 |

All 25 workspaces reconciled against expected balances and spending after the
runs. Retried writes retained the original transaction identity. Background CSV
counts matched the records visible during each export snapshot.

At the 25-user level, one-second metric samples observed API resident memory up
to 39.09 MiB, no canceled pool acquisitions, and oldest pending Asynq job age up
to 1.09 seconds. Connection-wait counters increased under bursts; this did not
violate the chosen latency budget. Samples are not continuous peak monitoring.
Asynq queue statistics do not measure the separate transactional outbox backlog.
The outbox still publishes batches of at most 50 every five seconds per process,
so a large import can take much longer to deliver all retained ledger events.

The [condensed measurement artifact](./benchmarks/2026-11-01-local-1000.json)
contains configuration, host limits, metric ranges, and container resource
samples. It excludes tokens, passwords, recovery codes, and financial records.
This 1,000-history run preceded the subsequent duplicate-review query optimization.

## Larger histories and the import fix

The first live attempt to build a 10,000-transaction history failed with HTTP
500 during duplicate-review preview after its first 1,000-row batch. This was
not reported as a passing benchmark. The old lookup repeated ledger/correction
joins for each incoming row and could exceed the API's ten-second request budget.

Duplicate lookup now materializes the incoming rows and eligible booked entries
once, separates reference/fingerprint/date-amount matches, deduplicates posting
matches, and keeps the original priority and three-candidate limit per CSV row.
Existing correction/reimport cases and a 10,000-record/1,000-row regression
verify reference matching and prevent new references with the same date/amount
from being falsely flagged. The regression fixture establishes PostgreSQL
statistics for its bulk-created historical data before its bounded read.

The first complete live rerun imported all 10,000 rows, had no unexpected errors
or throttling, and completed its background export. Its p95 was **736.22 ms**,
which failed the chosen 500 ms latency budget. The original runner stopped at
that budget failure before its final balance checks; do not treat that run as
fully reconciled. Its [failed-budget artifact](./benchmarks/2026-11-01-local-10000-latency-miss.json)
is retained.

A subsequent run against another newly seeded 10,000-row history passed:

| History | Simultaneous users | Requests | Successful requests/sec | p50 | p95 | Errors / HTTP 429 |
| --- | --- | --- | --- | --- | --- | --- |
| 10,000 | 1 | 42 | 2.79 | 7.75 ms | 60.22 ms | 0 / 0 |

This run verified final balance/spending, idempotent retries, and the background
CSV count. Per-endpoint p95 was 34.51 ms for Activity and 438.06 ms for spending.
The updated runner always performs final financial checks even after a workload
budget failure. See the [passing artifact](./benchmarks/2026-11-01-local-10000.json).

There was no further application change between these two completed large-history
runs. Their variation means the lower number is not a latency guarantee. Database
statistics/plans, cache state, and longer workloads need controlled investigation
before making a service commitment. The 10,000-row history was loaded through
the normal API in 1,000-row chunks; no manual `ANALYZE` command was run on the live benchmark database.

## Reproduce

Use an isolated app with synthetic data only. These commands create lasting
test workspaces; do not point them at a customer deployment.

```sh
python3 scripts/load_check.py \
  --base-url http://127.0.0.1:8088 --confirm-synthetic \
  --users 25 --rows 1000 --levels 1,5,10,25 --seconds 15 \
  --max-p95-ms 500 --metrics-env-file .env.app \
  --docker-project ledgerx-app --output tmp/load-baseline-1000.json

python3 scripts/load_check.py \
  --base-url http://127.0.0.1:8088 --confirm-synthetic \
  --users 1 --rows 10000 --levels 1 --seconds 15 \
  --max-p95-ms 500 --metrics-env-file .env.app \
  --docker-project ledgerx-app --output tmp/load-baseline-10000.json
```

The metrics token is read internally from the local configuration and never
written into the report. `--docker-project` is optional; it requires Docker
socket access. The saved artifact represents its particular run, not a promise
that a fresh run will have identical timings.

## Remaining measurements

Remeasure on the selected public host. Test a longer steady workload, mixed
histories, concurrent bulk imports/restores, no-reference duplicate-heavy CSVs,
slow object storage, restarts during work, and maximum-size workspace recovery.
The 50,000-transaction JSON limit has not been established as a safe concurrent
recovery workload. No capacity above the tested levels or production availability
target has been established. Monitor outbox lag/retention as well as Asynq jobs
before relying on event consumers at bulk-import volume.
