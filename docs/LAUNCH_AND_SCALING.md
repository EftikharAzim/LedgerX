# Public access and scalability assessment

Reviewed against the repository on 2026-11-01 UTC. This records current gaps and
acceptance criteria; it does not establish a public deployment or measured capacity.

## Current state

LedgerX is a working personal-spending MVP that currently requires a development
environment. No customer-facing hosted URL has been established in this project.
`localhost` refers to the visitor's own machine: sharing the owner's
`http://localhost:5173` address does not give another person access to the app.
A GitHub repository provides source code, not a running LedgerX service.

The system supports separate users and concurrent requests, but no representative
load test has established a safe number of concurrent users. Registered users,
active users, simultaneous requests, and transactions stored are different measures.
Do not advertise a user-capacity number based on the implementation language,
database connection count, functional tests, or backup format limits.

## How customers could access it

| Route | What the user receives | Cost and operational limits |
| --- | --- | --- |
| Local/self-hosted app | An installation they run on their own computer or server. | Uses their existing machine; setup and maintenance remain their responsibility. The current developer setup is too involved for a typical consumer. |
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
export storage on private connections. This is a proposed topology, not existing
deployment configuration. Vite's development proxy does not ship with the UI
build; production routing and SPA fallback must be configured explicitly.

## Release gaps and acceptance criteria

These operational priorities supplement the feature milestones in
[PRODUCT_PLAN.md](./PRODUCT_PLAN.md). They do not depend on recruiting customers.

| Priority | Work | Required evidence |
| --- | --- | --- |
| P0 — runnable package | Build and serve the UI alongside the API; run migrations in a controlled startup/release step; provide one documented full-stack start command. | A clean checkout starts successfully. A visitor can load and refresh a nested UI route, register, record spending, and download a report without running a second frontend development server. |
| P0 — durable state | Persist the export directory for a single instance, or select shared object storage; document database/Redis persistence and server backup/restore. | A container replacement preserves account data and previously generated report downloads. Restore the server database in an isolated environment. A user's financial JSON backup is not a substitute for server disaster recovery. |
| P0 — public access configuration | Configure HTTPS, a real signing secret, explicit browser origins, private database/Redis access, and protection of metrics/admin paths. Configure trusted-proxy handling for the chosen ingress. | Test from a separate device outside the owner's network. Verify login, refresh/logout, user isolation, and downloads over HTTPS. Verify distinct clients do not all share the proxy's authentication rate-limit bucket, and arbitrary forwarding headers cannot bypass limits. |
| P0 — measured baseline | Run the repeatable load scenarios below on recorded hardware and deployment settings. | Publish latency, error rate, resource use, queue backlog, and money-correctness results. State the largest workload that actually passed rather than estimating a user count. |
| P0 before real-record pilot | Finish usable account recovery and data-deletion controls; explain where records live and who operates backups. | A person can recover access and request/remove their data through documented, tested flows. Verify the recovery and retention behavior of the chosen deployment. These are still gaps in the current app. |
| P1 — release automation and usability | Automate the existing browser/accessibility/recovery checks; add clear start/import/restore onboarding and lazy-load heavier pages. | Release checks run in CI for the release branch/PR. First-load behavior is measured on a slower phone/network and the core tasks remain usable. |

The existing Compose file is development infrastructure: it has no frontend or
migration service, publishes PostgreSQL/Redis ports, uses example database
credentials, and does not mount the API's default export directory. Do not
publish this configuration unchanged as the customer service.

## What limits scaling today

| Component | Existing foundation | Current constraint / next change |
| --- | --- | --- |
| Ledger writes | PostgreSQL transactions, idempotent writes, and a transaction-scoped lock per user protect financial changes. | Different users can write concurrently. A long import/restore can delay that same user's other writes. Measure lock waits before changing this correctness boundary. |
| Database pool | One shared `pgxpool` serves the API and background work. | The code fixes the pool at **10 maximum / 2 minimum connections per process**. This is not a limit of 10 users. Measure connection waits; make sizing configurable and budget total connections across replicas and jobs. |
| Background work | Redis-backed Asynq queues and transactional outbox publishing already exist. | The same process runs the API, workers, outbox, and scheduler. Worker concurrency is **10**, sharing the database pool and CPU/memory with requests. Separate runtime roles or tune concurrency when measurements justify it. |
| Multiple instances | Shared PostgreSQL/Redis and outbox `SKIP LOCKED` claims provide useful building blocks. | Replicas need shared export storage and **exactly one scheduler** (`RUN_SCHEDULER=true` on one instance, `false` elsewhere). Each replica currently also adds workers and another database pool. Adding replicas unchanged is not a scaling plan. |
| Export files | Export CSV generation streams rows and supports local disk or S3-compatible storage. | Default local files are instance-specific and unmounted in Compose. A download routed to another instance may not find its file; persistent shared storage is needed before balancing across instances. |
| Imports and recovery | Atomic imports and restores have explicit input bounds. | CSV imports allow **1 MiB / 1,000 rows / 50 columns**. Financial JSON backups allow **32 MiB / 50,000 transactions / 150,000 postings** and are materialized in memory. These are workflow limits, not a whole-database transaction limit or demonstrated safe concurrent workload. |
| Operations | Health/readiness endpoints, structured logs, and HTTP/database metrics exist. | No measured service capacity, availability history, or load-based alert thresholds are established. Monitor database waits, memory, queue age, storage growth, and backup recovery before making service commitments. |

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

First make the complete app reproducible and persistent on one machine. Next
measure its baseline and verify a zero-spend demo hosting option against the
whole stack. A public demo can then make the UI accessible while account
recovery, deletion, and server recovery are completed for a real-record pilot.
Keep a single API instance initially; add infrastructure complexity only when
measurements identify a bottleneck. Customer interviews remain necessary to
validate demand, but do not block these engineering improvements.

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
