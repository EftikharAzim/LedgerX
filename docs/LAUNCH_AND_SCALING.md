# Public access and scalability assessment

Reviewed against the repository on 2026-11-01 UTC, then updated after implementation.
Local verification and baseline measurements do not establish a public deployment
or a supported-user capacity promise.

## Current state

LedgerX is a working personal-spending MVP with a Docker-only local package.
See [running and deployment](./RUNNING_AND_DEPLOYMENT.md) for one-command startup,
persistent storage, server recovery, and release checks. No customer-facing hosted URL has been established in this project.
`localhost` refers to the visitor's own machine: sharing the owner's
`http://localhost:5173` address does not give another person access to the app.
A GitHub repository provides source code, not a running LedgerX service.

The system supports separate users and concurrent requests. A repeatable local
load runner now measures synthetic workloads; see the [baseline report](./SCALABILITY_BASELINE.md).
No production load test has established a safe number of concurrent customers. Registered users,
active users, simultaneous requests, and transactions stored are different measures.
Do not advertise a user-capacity number based on the implementation language,
database connection count, functional tests, or backup format limits.

## How customers could access it

| Route | What the user receives | Cost and operational limits |
| --- | --- | --- |
| Local/self-hosted app | An installation they run on their own computer or server. | Uses their existing machine; setup and maintenance remain their responsibility. A Docker-only package now exists; it still requires installation and an always-running machine. |
| Temporary shared demo | An HTTPS link to a deliberately exposed demo instance using dummy records. | A tunnel or hosted free tier may work within its current terms. A tunnel to a laptop depends on that laptop and connection staying online. This is a demonstration, not an availability commitment. |
| Hosted service | A stable HTTPS address where customers register and return to their own records. | Requires the frontend, Go API/workers, PostgreSQL, Redis, and durable export storage. Free-tier eligibility, persistence, sleep behavior, quotas, and overage rules must be checked together before choosing a provider. |

Keep the owner's zero-spend constraint. Use a provider-issued address before
buying a domain; use the existing keyword categorizer without a paid AI key.
No provider's current free plan has been verified for this complete stack in this
assessment. Free frontend hosting alone does not host the Go API or its data.
If a complete persistent free stack is unavailable, keep the deployment a
limited demonstration rather than promising a free always-on customer service.

The simplest initial hosted topology is one HTTPS origin serving the built UI
and forwarding `/auth` and `/v1` to one API process, with PostgreSQL, Redis, and
export storage on private connections. The local package now implements this topology. An optional HTTPS ingress
template is prepared; no public deployment has been made. Vite's development proxy does not ship with the UI
build; production routing and SPA fallback must be configured explicitly.

## Release gaps and acceptance criteria

These operational priorities supplement [PRODUCT_PLAN.md](./PRODUCT_PLAN.md).
They can proceed without customer recruitment. Current implementation status:

| Priority | Outcome | Status and remaining evidence |
| --- | --- | --- |
| P0 — runnable package | One command serves built UI/API, applies migrations, and waits for readiness. | Implemented. Fresh isolated stacks and nested-page refresh were verified. The developer Compose file remains separate. |
| P0 — durable state | Persist reports and verify server recovery. | Implemented. Container replacement preserved balances and existing CSV downloads. PostgreSQL/Redis/export-volume recovery passed; occupied restore destinations are rejected. Automatic off-server backups and retention are still operator work. |
| P0 — public access | HTTPS, private dependencies, signing secrets, origins, and trusted ingress. | Local package and optional HTTPS configuration implemented. Proxy spoofing/distinct-client tests and Caddy config validation passed. Public hosting, DNS, certificate issuance, and external-device access remain unverified. Owner has no hosting account. |
| P0 — measured baseline | Report latency, failures, resource pressure, queues, and financial correctness. | Runner implemented; local workloads and their limits are in [SCALABILITY_BASELINE.md](./SCALABILITY_BASELINE.md). Remeasure on the selected host before publishing capacity claims. |
| P0 — account safeguards | Usable recovery, deletion, and clear retention behavior. | Saved-code recovery, password changes, session invalidation, and confirmed account deletion implemented and tested. Users must save a code before losing access; no email-based recovery or inbox verification exists. Store versions and backup retention need deployment-specific handling. |
| P1 — release automation and usability | Repeatable browser checks, start/import/restore choices, and page loading. | Implemented. Local release flows cover 93 accessibility states with zero reported violations. CI is configured; local checks are not remote CI results. The main JS entry bundle fell from 704.53 kB to about 417 kB minified; lazy chunks load separately and real-device performance remains unmeasured. |

Use `deploy/compose.app.yml` for the complete package. The older
`deploy/docker-compose.yml` remains development infrastructure: no frontend or
migration service, published database/Redis ports, example credentials, and no
API export mount. Do not expose that developer configuration unchanged.

## What limits scaling today

| Component | Existing foundation | Current constraint / next change |
| --- | --- | --- |
| Ledger writes | PostgreSQL transactions, idempotent writes, and a transaction-scoped lock per user protect financial changes. | Different users can write concurrently. A long import/restore can delay that same user's other writes. Measure lock waits before changing this correctness boundary. |
| Database pool | One shared `pgxpool` serves the API and background work. | Defaults are **10 maximum / 2 minimum connections per process**, now configurable with `DB_MAX_CONNS`/`DB_MIN_CONNS`. This is not a limit of 10 users. Pool wait/cancellation metrics expose pressure; budget connections across replicas and jobs. |
| Background work | Redis-backed Asynq queues and transactional outbox publishing already exist. | The same process runs the API, workers, outbox, and scheduler. Worker concurrency is configurable: **4 in the app package**, 10 in host defaults, sharing database/CPU/memory with requests. Separate runtime roles when measurements justify it. |
| Multiple instances | Shared PostgreSQL/Redis and outbox `SKIP LOCKED` claims provide useful building blocks. | Replicas need shared export storage and **exactly one scheduler** (`RUN_SCHEDULER=true` on one instance, `false` elsewhere). Each replica currently also adds workers and another database pool. Adding replicas unchanged is not a scaling plan. |
| Export files | Export CSV generation streams rows and supports local disk or S3-compatible storage. | The app package mounts a persistent local export volume. Separate instance disks still do not share files; use shared storage before balancing downloads across replicas. Active exports also hold a database connection and a deletion lease. |
| Imports and recovery | Atomic imports and restores have explicit input bounds. | CSV imports allow **16 MiB / 50,000 rows / 50 columns**. Financial JSON backups allow **32 MiB / 50,000 transactions / 150,000 postings** and are materialized in memory. These are workflow limits, not a whole-database transaction limit or demonstrated safe concurrent workload. |
| Operations | Health/readiness endpoints, structured logs, and HTTP/database metrics exist. | Local synthetic measurements exist; hosted service capacity, availability history, and load-based alert thresholds remain unestablished. Pool and queue counts/age are now exposed through protected metrics. Monitor database waits, memory, queue age, storage growth, and backup recovery before making service commitments. |

Current functional, browser, accessibility, and race-test results support the
tested behavior. They do not establish production availability or concurrent-user
capacity. In particular, the tested 1,000-record workspace restore is not a
50,000-record restore benchmark or a multi-user load test.

## Free, repeatable capacity check

Run this against an isolated database with synthetic users and records, never
the owner's financial workspace. Use a local script or a free load-testing tool;
no paid service or real customers are required.

1. Record CPU/RAM, software versions, process count, database limits, rate-limit
   settings, storage backend, and whether services share a machine. Establish
   task-specific latency budgets before judging a run.
2. Seed histories of 1,000, 10,000, and 50,000 transactions as separate data-size
   scenarios. Treat restore format caps separately from ordinary database size.
3. Run 1, 5, 10, then 25 simultaneous simulated users with realistic pauses:
   login, overview, paginated Activity/search, add spending, and review reports.
   These are proposed test sizes, not supported-user claims. Pace authentication
   and API requests within configured limits; report HTTP 429s separately from
   application failures rather than silently disabling protection.
4. Repeat ordinary browsing while another user imports 1,000 CSV rows or exports
   all history. Separately test backup/restore memory and duration near supported
   bounds, including multiple simultaneous operations and same-user contention.
5. Measure p50/p95 request latency, unexpected error rate, throughput, CPU/RAM,
   database connection/lock waits, and queue backlog/drain time. Reconcile
   balances, category totals, retries, and export counts after each run.
6. Re-test container replacement and worker interruption. Verify recovery, no
   duplicate ledger writes, and continued access to completed exports. Stop
   increasing load when latency, correctness, or resource limits fail.

Report the passing configuration and workload, then remeasure on the selected
host. A laptop benchmark does not establish the capacity of a smaller free host.

## Recommended next delivery

Use the complete package and local baseline as the starting point. Next verify
a zero-spend hosting option for the **whole** persistent stack and test public
HTTPS from another device. No public URL or provider account exists yet. Use
dummy records for the first shared demo; define backup/retention operations
before a real-record pilot. Larger-data, concurrent import/restore, sustained
load, and deployment failure scenarios still need measurement.

Keep one API instance initially. Account/category management is implemented;
direct customer observation remains a gap after these operational improvements. The
current evidence does not validate demand or establish product-market fit.

## Source references

- [Compose](../deploy/docker-compose.yml), [API Dockerfile](../deploy/Dockerfile),
  [Vite development proxy](../ledgerx-ui/vite.config.ts), and
  [browser API client](../ledgerx-ui/src/api/client.ts).
- [Database pool](../internal/repo/db.go),
  [process wiring and scheduler](../cmd/ledgerx/main.go),
  [worker concurrency](../internal/worker/snapshot_worker.go), and
  [outbox publishing](../internal/worker/outbox.go).
- [Per-user write lock](../internal/repo/queries/imports.sql),
  [storage selection](../internal/storage/s3.go),
  [CSV bounds](../internal/importing/csv.go), and
  [workspace format bounds](../internal/workspace/backup.go).
- [Authentication rate limiting](../internal/transport/http/ratelimit.go),
  [router/proxy behavior](../internal/transport/http/router.go), and
  [CORS configuration](../internal/transport/http/cors.go).
