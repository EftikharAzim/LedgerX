# Engineering review and cleanup

Reviewed 2026-11-02 against `4c1b7d9` on `codex/modern-ui`.

## Scope and architecture

This repository-wide review combines architecture/source inspection with static
analysis, real PostgreSQL/Redis tests, frontend tests and packaged-browser
regressions. It covers HTTP, services, SQL/migrations, workers, storage,
authentication, frontend state/forms, deployment scripts and CI. Financial
writes, retries, recovery and resource lifetimes received detailed inspection.
Generated SQLC files were treated as outputs, not cleanup targets.

Keep the existing modular monolith: PostgreSQL owns recorded money, service
transactions own financial changes, and Redis supports derived data and jobs.
There is no evidence that microservices, another framework or a generic
repository abstraction would improve current reliability.

## Findings addressed

| Area | Finding | Change |
| --- | --- | --- |
| Exact money | Ordinary entries/transfers and standalone reversals could exceed the exact range already enforced by imports/plans. | Reuse shared checks for amounts and resulting real-wallet balances. Failed writes roll back postings, outbox rows and retry claims. Balance/summary reads reject unsupported totals. |
| Posted dates | Transfers could post future money. | Apply the same posted-date validation as income/expenses. Preserve backdated manual history, including records before a wallet's starting balance; the existing import regression explicitly exercises this workflow. |
| Summary cache | Malformed JSON could become a zero summary. Concurrent misses could refill stale values after invalidation. | Validate cache shape, range and totals; use WATCH for fills and an expiring invalidation marker that changes absent keys too. SQL remains the fallback during cache failures. |
| Export files | Failed overwrites truncated existing downloadable reports. | Stage a private file beside the destination; atomically rename after successful write/close. On failure, retain the previous export and remove temporary files. Preserve root confinement and traversal/symlink protection. |
| Category types | Confirmation and suggestions could disagree with income/expense/refund and parent-type rules. | Share filtering between services, draft hints and queued suggestions; validate confirmation and filter its UI choices. Refresh affected ledger views afterward. |
| JSON requests | Decoding was duplicated; some old endpoints had no byte cap, and a valid prefix could hide a second document or oversized tail. | Share single-document decoding. Preserve existing limits/unknown-field policies; cap previously unbounded account requests at 8 KiB and transaction/transfer/category-confirmation requests at 16 KiB. |
| Process lifecycle | Context workers were not joined before shared clients closed; component failures did not promptly cancel them. Some Redis/task clients were never closed. | Cancel on both shutdown paths, join owned context workers, close clients, force-close HTTP connections after a failed drain, and release a pool after failed startup ping. Add body/idle connection timeouts. |
| Rate limiting | Visitor bucket keys persisted indefinitely. | Expire both keys after an idle bucket would fully refill; reuse one script definition. Existing unused legacy keys are not globally flushed. |

Cleanup also consolidates browser downloads, moves shared money helpers out of
import/report files, normalizes Go formatting, and adds CI formatting and
`go mod tidy -diff` checks without silently modifying dependency declarations.
The budget browser check now waits for refreshed cards before asserting their
count, removing an asynchronous timing race without relaxing the assertion.

No schema migration, dependency upgrade, ledger rewrite or backup-format change
is required. Existing idempotency hashes, routes and valid request shapes remain
compatible. New rejections concern invalid/unsupported money, future postings,
malformed JSON and incompatible category assignments.

## Verification

- 254 Go test cases pass with race detection against disposable PostgreSQL and
  Redis, including existing tests and regression cases for these findings.
- Go lint/formatting, `go mod tidy -diff`, frontend tests, ESLint and the production
  build pass.
- All 66 browser/accessibility states pass. The release gate covers imports,
  wallets, categories, budgets, reports,
  recurring plans, downloads, authentication and workspace recovery.
- A separate package check verifies container replacement and server
  backup/restore, including refusal of a populated restore destination.

The frontend dependency audit reports zero known vulnerabilities. Refreshing the
Go vulnerability database is blocked here by HTTP 403 for `vuln.go.dev`; it is
not counted as passing. The unchanged dependency graph and historical evidence
are recorded in [DEPENDENCY_AUDIT.md](DEPENDENCY_AUDIT.md). CI retains its Go
vulnerability gate. Live S3 and paid model providers were not exercised; tests
use local storage and the free keyword categorizer.

## Engineering rules

1. Keep HTTP concerns in handlers and financial rules/transaction boundaries in
   services. Share rules across direct, imported, planned and correction paths.
2. Preserve the per-user ledger lock, atomic retry claim and append-only
   postings/outbox boundary. Keep correctness out of React and Redis.
3. Parse decimal inputs exactly; keep currencies separate. Validate arithmetic
   before exposing JavaScript numbers. Plans require explicit confirmation.
4. Edit SQLC query sources and regenerate outputs. Move feature SQL incrementally
   while preserving ownership and lock semantics.
5. Use narrow dependency interfaces where they enable meaningful failure/race
   tests; avoid abstractions added only for symmetry or implementation-mirroring
   tests.
6. Preserve retry identities and backup compatibility. Add regressions for
   financial/concurrency fixes; use existing browser flows for simple UI changes.
   Keep private customer records out of fixtures and logs.
7. Use disposable test services. Run relevant checks and the full release gate
   for changes spanning money, authentication, imports or recovery.

## Remaining work

| Priority | Follow-up | Reason |
| --- | --- | --- |
| Before substantial feature expansion | Move inline SQL from larger services into feature repository queries; split large import/schedule views when changing those workflows. | Reduce review/navigation cost through small, behavior-preserving changes. |
| Before stronger cache-consistency promises | Add durable invalidation/reconciliation for cancellation or Redis outages. | WATCH fixes stale concurrent fills, but post-commit invalidation is still best effort with a five-minute TTL. Database-derived balances/reports remain authoritative. |
| Before larger recurring workloads | Add refresh retry/backoff/fairness; measure batch throughput and queue age. | Refresh is bounded to 100 due users per batch; one failed user's schedule refresh rolls back together. |
| Before public production commitments | Complete hosting, monitoring, recovery drills, realistic load tests and direct customer validation. | Local tests do not establish a public capacity/SLA. See [LAUNCH_AND_SCALING.md](LAUNCH_AND_SCALING.md). |

An image/OS vulnerability scan and live external-provider tests remain separate
release work. This review does not claim those checks or an absence of every bug.
