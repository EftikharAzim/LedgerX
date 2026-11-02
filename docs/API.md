# LedgerX API Reference

API amounts are integers in the currency's minor units (USD cents; whole JPY).
All timestamps are RFC 3339. CSV exports additionally include readable decimal amounts.

JSON request bodies must contain one complete document; trailing whitespace is
allowed, while a second document or trailing garbage is rejected. Existing
endpoint limits and unknown-field policies apply. Account creation is limited
to 8 KiB; transaction, transfer and category-confirmation bodies to 16 KiB.
Import and workspace-upload limits are unchanged. Monetary amounts and resulting
wallet balances must remain within ±9,007,199,254,740,991 minor units. Unsupported
values return 422. Transfers, like income/expenses, cannot post future money.
Backdated manual history remains supported, including dates before a wallet's
starting balance. Category confirmation enforces income/expense/refund types
and parent inheritance without changing recorded amounts.

## Authentication

`register`/`login`/`refresh` return:

```json
{ "access_token": "<JWT>", "refresh_token": "<opaque>", "token_type": "Bearer", "expires_in": 900 }
```

The **access token** is a short-lived JWT (default 15 min) sent as
`Authorization: Bearer <access_token>` on every `/v1/*` call. The **refresh
token** is long-lived (default 30 days), opaque, and stored server-side only as
a SHA-256 hash. Use it to mint new access tokens without re-entering
credentials. See the rotation/reuse-detection flow in [SEQUENCES.md](./SEQUENCES.md).

| Method | Endpoint | Body / params | Notes |
| --- | --- | --- | --- |
| POST | `/auth/register` | `{email, password}` | password ≥ 8 chars; `201` with token pair; `409` if email taken |
| POST | `/auth/login` | `{email, password}` | `200` with token pair |
| POST | `/auth/refresh` | `{refresh_token}` | Rotates: returns a new pair and revokes the old refresh token. `401` if unknown/expired. Reusing an already-rotated token revokes the user's whole token chain (theft response) |
| POST | `/auth/logout` | `{refresh_token}` | Revokes the refresh token. `204`; idempotent |
| POST | `/auth/recover` | `{email, recovery_code, password}` | Consumes a previously saved code, changes the password, and ends all sessions; `204` |

`/auth/*` is rate-limited per client IP.

Password changes/recovery increment the user's credential generation. Every
protected request checks it, so old access tokens immediately stop working.
Account deletion also ends access. See [ACCOUNT_SECURITY.md](./ACCOUNT_SECURITY.md)
for saved-code prerequisites, retention limits, and the authenticated
`/v1/security`, `/v1/security/recovery-code`, `/v1/security/password`, and
`/v1/security/account` endpoints. No email reset service is configured.

Token issuance, rotation, logout, and reuse revocation serialize on the user's
database row. Refresh handlers reread token state after taking that lock in a
`READ COMMITTED` transaction. Reusing an ancestor therefore also revokes a
descendant created by an overlapping rotation before the lock was acquired.

## Users

| Method | Endpoint | Notes |
| --- | --- | --- |
| GET | `/v1/users/me` | Current user's id and email |

## Accounts

Each user automatically has one `external` account representing the outside
world; it absorbs the offsetting leg of income/expense transactions. It cannot
be created or listed away — filter on `kind` client-side.

| Method | Endpoint | Body / params | Notes |
| --- | --- | --- | --- |
| POST | `/v1/accounts` | `{name, currency, opening_balance_minor?, opening_balance_at?}` | `201`; name unique per user; optional `Idempotency-Key` |
| GET | `/v1/accounts` | `?limit=&offset=` | Includes the `external` account (`kind` field) |
| PATCH | `/v1/accounts/{id}` | `{name?, active?, expected_name, expected_active}` | Rename, archive (`active: false`) or restore; owner only; currency is immutable |
| GET | `/v1/accounts/{id}/balance` | | Snapshot + delta; owner only |
| GET | `/v1/accounts/{id}/summary` | `?month=YYYY-MM` | Inflow/outflow/net; cached 5 min; owner only |
| GET | `/v1/accounts/{id}/transactions` | `?limit=&cursor=` | Newest first; response `{entries, next_cursor}`; pass `next_cursor` back to page |

History entries include `is_transfer` (true when another normal account is on
the other side), optional `reversal_of` (the original transaction ID), and
optional `reversed_by` (the correcting transaction ID). These relationships
come from the ledger, not the note text. A transfer reversal has both
`is_transfer: true` and `reversal_of` set.

Monthly summaries use UTC calendar months and include transfers and reversals.
All amounts are integer currency minor units: USD 1.01 is `101`, while JPY
1,250 is `1250`.

Account creation accepts a signed starting balance (default zero). A nonzero
balance requires an RFC 3339 `opening_balance_at` no later than now (one-minute
clock tolerance). Name, account, starting postings, and outbox commit atomically.
The starting amount is limited to JavaScript-safe integer minor units. Repeating
the same creation key and payload returns the same account; changing the payload
under that key returns `409`. Name/currency are normalized before comparison.
Starting balances affect ledger balances but are excluded from spending/income.

Account updates accept at least one of `name` or `active`, plus the name and
active state last reviewed by the caller. Names are trimmed and limited to 80
characters. A competing change returns `409`; reload and review the current
details before retrying. Repeating an already-applied change returns `200`.
Unknown fields, including `currency`, return `400`; invalid input returns `422`.
Absent, foreign and external accounts return `404`.

Archiving retains all balances, reports, history and pending plans. It prevents
new entries, transfers and plan confirmation. Active recurring schedules must
first be paused or ended (`409` otherwise); paused schedules cannot resume until
the account is restored. Restoring never resumes schedules or posts plans.
Metadata updates serialize with financial writes and do not create ledger
entries. Archived account names remain reserved under the normal uniqueness rule.

## Spending and unified activity

`GET /v1/spending` accepts `currency=USD` plus the same period inputs as exports
(`type=monthly&month=YYYY-MM`, `weekly&week_of=YYYY-MM-DD`, `yearly&year=YYYY`,
`custom&start_date=...&end_date=...`, or `type=all`). A valid currency is required.
It returns `income_minor`, `spending_minor`, `net_minor`, `spending_count`,
`categories: [{category_id, name, spending_minor, count}]`, `time_zone: "UTC"`,
and `correction_policy: "exclude_reversed"`. Uncategorized entries have category
ID zero. The query is scoped to the authenticated user and one currency.

Spending excludes internal transfers and starting balances. Explicit refunds
reduce spending in the period received and may produce a negative category or
period total. Reversed originals and their reversals are omitted entirely,
restating the original spending period; they remain in account balances/history
and CSV exports. Net is income minus spending, not a safe-to-spend forecast.
The older account summary endpoint retains its gross movement semantics.

`GET /v1/activity` supports optional `search` (literal substring, max 120
characters), `account_id`, `category_id` (zero = uncategorized), `currency`,
`type` (`income`, `expense`, `refund`, `transfer`, `opening_balance`, `reversal`),
`start_date` and `end_date` (both required when filtering dates, inclusive UTC),
`limit` (1–100, default 30), and `cursor`. Filters apply to the full history
before pagination. The uncategorized filter covers uncanceled income, purchases,
and refunds; starting balances, transfers, and reversals do not need categories.
A transfer appears once with its outgoing account and
`counterparty_account_id` / `counterparty_account_name`; an account filter matches
either side. Entries also include `account_name`, `entry_type`, `activity_type`,
and the existing category/reversal metadata. Pagination returns
`{entries, next_cursor}` using timestamp and transaction ID. Filtered foreign
accounts return `404`.

## Transactions (double-entry)

Every transaction is a header plus ≥2 postings that sum to zero — enforced
both in the service layer and by a deferred database trigger.

| Method | Endpoint | Body | Notes |
| --- | --- | --- | --- |
| POST | `/v1/transactions` | `{account_id, amount_minor, currency, occurred_at, note?, category_id?, entry_type?}` | Income (+) / expense (−), or explicit positive refund; the external account takes the offsetting leg. `201` with header + postings |
| POST | `/v1/transactions/{id}/reverse` | — | Append-only correction: creates a new transaction negating the original's postings, linked via `reversal_of`. `409` if already reversed or if `{id}` is itself a reversal |
| POST | `/v1/transactions/{id}/category` | `{category_id}` | Confirm a category (the human side of AI suggestion). `category_id` must be one of your categories; `422` otherwise. Clears any AI suggestion |
| POST | `/v1/transfers` | `{from_account_id, to_account_id, amount_minor, currency, occurred_at, note?}` | `amount_minor` must be positive; both accounts must be yours and share the currency |

All three endpoints accept an `Idempotency-Key` header. Replaying the same key
with the same payload returns the original response; a different payload under
the same key returns `409`. The key claim, ledger write, outbox event, and
cached response commit in one database transaction.

An optional positive `category_id` must belong to the caller. It is confirmed
atomically with the ledger write, returned in the response, and included in
idempotency comparisons. Selected categories skip AI suggestion generation.
Omitting the category keeps the earlier request/replay behavior.

`entry_type` may be omitted/`standard` or `refund`; a refund must be positive.
Only account creation can create `opening_balance` entries. Refund classification
is part of the idempotency request identity; changing it under the same key
returns `409`. The default classification preserves deployed request hashes.

`/auth/*` is rate-limited per client IP; `/v1/*` per user. Limits surface in
`X-RateLimit-*` headers; when the limiter's Redis is unavailable requests fail
closed with `503`.

Validation failures return `422` (unbalanced, zero amount, inactive account,
currency mismatch), unknown/foreign accounts return `404`.

## Categories

| Method | Endpoint | Body | Notes |
| --- | --- | --- | --- |
| POST | `/v1/categories` | `{name}` | Create a category; unique per user (case-insensitive). `409` on duplicate |
| POST | `/v1/categories/suggest` | `{note, currency, kind}` | Read-only free hint for an expense/income/refund draft; explanation and explicit acceptance in UI |
| GET | `/v1/categories` | | List your categories |

New income/expense transactions that carry a note are asynchronously assigned a
suggested category by an LLM (see [AI_CATEGORIZATION.md](./AI_CATEGORIZATION.md)).
The suggestion appears on entries as `suggested_category_id` + `suggestion_score`;
confirm it via `POST /v1/transactions/{id}/category`. The AI never sets the
confirmed `category_id` directly.

The draft helper accepts a note of 3–240 characters, a valid uppercase currency,
and `kind: expense | income | refund`. It returns `{category_id, category_name,
source, reason, supporting_records, matching_records}`; category ID zero means no
clear recommendation. Sources are `history`, `keyword`, `ambiguous`, and `none`.
It checks up to 20 recent confirmed same-note/currency/kind records for the caller,
excluding canceled entries and transfers. At least 80% must agree; with no history,
one whole-word category-name match may be suggested. It never calls an external
model or writes transaction/category data. Responses use `Cache-Control: no-store`.

## Exports

| Method | Endpoint | Body / params | Notes |
| --- | --- | --- | --- |
| POST | `/v1/exports` | Period query parameters below | `202`; CSV generated by a background worker |
| GET | `/v1/exports/{id}/status` | | Owner only |
| GET | `/v1/exports/{id}/download` | | Owner only; `409` until status is `done` |

Reports include the caller's normal accounts and use inclusive UTC calendar
dates. `month` without a `type` remains supported for existing clients.

| Report | Query parameters |
| --- | --- |
| Monthly | `type=monthly&month=2026-10` |
| Weekly | `type=weekly&week_of=2026-10-28` (the containing Monday–Sunday week) |
| Yearly | `type=yearly&year=2026` (January 1–December 31) |
| Custom | `type=custom&start_date=2026-10-01&end_date=2026-10-15` |
| All time | `type=all` (every existing entry, including future-dated records) |

Invalid/reversed dates, unsupported types, or mixed period parameters return
`400`. Status metadata includes `report_type`, `start_date`, and `end_date`;
all-time reports omit date bounds, while monthly reports retain `month`.
The worker reads the stored range, streams a single consistent database query
to a temporary CSV file, then saves it to the configured export storage.

The original columns remain first: `transaction_id,account_id,account_name,
category_id,amount_minor,currency,occurred_at,note`. Additional columns are
`category_name,amount,type,reversal_of,reversed_by,import_id,date_only,correction_of,corrected_by`. `amount` is an exact decimal
string using the currency's precision (for example USD `1.01`, JPY `1250`).
User-provided text that could be interpreted as a spreadsheet formula is prefixed
with an apostrophe. CSV contains all normal-account postings, including both
sides of transfers and canceled/correcting entries. Use the type and reversal
columns when interpreting movement; it is not a spending-only report or full backup.

## CSV imports and current balance checks

Every endpoint below is authenticated and owner-scoped, with `Cache-Control:
no-store`. CSV/money formats and recovery behavior are detailed in
[IMPORTS_AND_BALANCE_CHECKS.md](./IMPORTS_AND_BALANCE_CHECKS.md).

| Method | Endpoint | Body / behavior |
| --- | --- | --- |
| POST | `/v1/imports/inspect` | `{csv, delimiter?}` → headers, separator, row count, first five samples, source groups and unambiguous date-format suggestion; no persistence |
| GET | `/v1/accounts/{id}/import-context` | Current balance, posting revision, active starting component, tracking date, review token |
| POST | `/v1/imports/preview` | `{account_id, filename, csv, delimiter, source_group?, mapping, starting_balance}` → `201` immutable staged review; no postings |
| GET | `/v1/imports` | Latest ten session metadata/receipts for the caller |
| GET | `/v1/imports/{id}` | Pending normalized preview or durable completed receipt |
| POST | `/v1/imports/{id}/commit` | `{rows: [sourceLineNumbers], duplicate_approvals: [sourceLineNumbers]}` → receipt, atomically |
| GET | `/v1/accounts/{id}/balance-checks` | Latest five checks plus current posting revision, recorded balance, and currency |
| POST | `/v1/accounts/{id}/balance-checks` | `{actual_minor, note?}` → `201` comparison snapshot; optional `Idempotency-Key`; zero/negative actual allowed |

Mapping column positions are zero-based integers; omit optional mappings.
Required choices: `amount_mode: signed | debit_credit`, `date_format: iso |
day_first | month_first`, `number_style: dot | comma`, and `sign_convention:
income_positive | expense_positive`. Map `date` and `amount` or `debit`/`credit`;
optional fields: `note`, `category`, `currency`, `type`, `reference`, `exclude_report`. Active fields
must use distinct columns. Starting plan: `{mode: existing | set, amount_minor,
date: YYYY-MM-DD | ""}`. Blank set date uses the earliest valid source date.

Commit accepts only valid server-staged rows and explicit duplicate approvals.
The stage ID and canonical selection form its retry identity: the same selection
replays the receipt, a changed selection after completion returns `409`.
Account changes since preview return `409`; expired previews `410`;
invalid rows/selections/range `422`; unknown or foreign resources `404`.
Balance-check key reuse with a different body returns `409`.

History and Activity add optional `import_id` and `import_date_only` fields.
`GET /v1/activity?import_id=ID` verifies import ownership and filters across the
whole matching history. It shows the selected source entries; starting balance
corrections remain in account history. Exports append origin/date semantics to
the existing columns. A full ledger report is intentionally rejected as a bank
CSV import to avoid replaying both transfer legs or canceled money.

## Reviewed transaction editing

All editing endpoints require ownership and return `Cache-Control: no-store`.

| Method | Endpoint | Behavior |
| --- | --- | --- |
| GET | `/v1/transactions/{id}/edit-context` | Editable draft and `source_token`; canceled originals/reversals return `409` |
| POST | `/v1/transactions/{id}/correction-preview` | `{draft, source_token}` → before/after fields, affected balances, `preview_token`; no money writes |
| POST | `/v1/transactions/{id}/corrections` | `{draft, source_token, preview_token}` plus required `Idempotency-Key` → `201` correction receipt |
| GET | `/v1/transactions/{id}/corrections/result` | Same `Idempotency-Key` header recovers only that caller's matching write attempt |

Draft fields: `account_id`, optional `to_account_id`, signed `amount_minor`,
`currency`, `entry_type` (`standard`, `refund`, `transfer`, `opening_balance`),
`occurred_at`, `date_only`, `note`, and `category_id` (zero clears). Transfer amounts
are negative on the source; starting money may be zero/negative. Currency stays
fixed. Transfer/starting kinds stay fixed; eligible normal transactions can
change between standard income/expense and positive refunds. Unchanged drafts
and invalid input return `422`; source/account changes require a refreshed
preview (`409`). Same-key same-body replay returns the original receipt.

History/Activity add `correction_of` and `corrected_by`; Activity accepts an
owner-checked `transaction_id` filter. Correction chains retain import origin
and explicit calendar-date semantics. CSV appends the same correction links.

## Financial workspace backup and restore

| Method | Endpoint | Behavior |
| --- | --- | --- |
| GET/POST | `/v1/workspace/backup` | Private `application/json` attachment; POST optionally accepts `{appearance: {accountID: {icon, color}}}` |
| GET | `/v1/workspace` | `empty`, all normal-account metadata, and latest ten restore summaries/receipts |
| POST | `/v1/workspace/restores/preview?filename=...` | Raw version-1 workspace JSON, max 32 MiB → `201` validated summary/checksum/review ID |
| GET | `/v1/workspace/restores/{id}` | Owner-only summary or completed receipt; no staged financial payload |
| POST | `/v1/workspace/restores/{id}/commit` | `{checksum}` → `201` receipt; atomic, empty workspace only, durable replay by review ID/checksum |

Backups use `format: ledgerx.workspace`, `version: 1`, `exported_at`, typed `data`,
and canonical `sha256`. Authentication/retry secrets, unsaved previews, generated
reports, jobs, and derived snapshots/cache are excluded. All authoritative
financial rows, imports, corrections, checks, timestamps, and optional account
appearance are retained. Restore remaps IDs and historical posting revisions.
It never overwrites/merges a populated workspace (`409`). Invalid/corrupt files
return `422`; oversized uploads `413`; expired reviews `410`; foreign resources
`404`. Responses use `Cache-Control: no-store`.

See [WORKSPACE_RECOVERY.md](./WORKSPACE_RECOVERY.md) for file limits, snapshot
semantics, credential exclusions, and recovery behavior.

## Operational endpoints

| Method | Endpoint | Notes |
| --- | --- | --- |
| GET | `/healthz` | Liveness |
| GET | `/readyz` | Checks Postgres + Redis |
| GET | `/metrics` | Prometheus |
| POST | `/admin/snapshot/run` | Requires `X-Admin-Token` matching `ADMIN_TOKEN`; disabled when unset |

## Planned activity

`GET /v1/plans?offset=0` returns up to 50 pending entries and `next_offset`
(zero means the end). Results are owner-scoped, ordered by due date and ID, and
include account/category labels, exact minor units, original date semantics and
report-exclusion flags. Plans do not contribute postings or report totals.

`POST /v1/plans` requires `Idempotency-Key` and
`{account_id, amount_minor, currency, occurred_at, note?, category_id?,
entry_type?, date_only?, exclude_from_reports?}`. The date must be future, the
account active/owned and currency matching. Standard and positive refund types
are supported. A repeated identical request returns the same plan; changed
payload with the same key returns 409.

`POST /v1/plans/{id}/post` takes `{date: "YYYY-MM-DD"}` for the actual date in
UTC, no later than today. It creates the balanced transaction and import origin
(if present) atomically. Repeated confirmation returns the original transaction
ID. `POST /v1/plans/{id}/cancel` closes a pending plan without postings; repeated
cancellation is harmless. Confirming a canceled plan or canceling a posted plan
returns 409. Missing/foreign plans return 404. There is no automatic posting.

Manual transaction creation rejects future dates with 422 and directs clients
to plans. Future CSV rows are routed to plans automatically; import receipts
separate `transaction_ids` from `planned_ids` and include `planned_count`,
`excluded_report_count` and `other_source_count`. Excluded records still affect
balances; their flag is retained in Activity/details, corrections and exports.
CSV exports contain posted activity; financial JSON backups also contain plans.

## Category organization, budgets and interactive reports

`POST /v1/categories` accepts `name`, optional `parent_id` (zero for root),
`kind` (`any`, `income`, `expense`), and `icon`. `GET /v1/categories` includes
archive/type/parent/icon metadata and omits deleted categories.
`PATCH /v1/categories/{id}` accepts partial edits of these fields and `archived`.
`GET /v1/categories/{id}/impact` returns record/plan/child/budget counts and a
`review_token`. `DELETE /v1/categories/{id}` takes
`{replacement_id: 0, review_token}`; zero means Uncategorized. Stale reviews
return 409. Children and attached budgets must be resolved first. Postings and
source provenance are untouched.

`GET /v1/budgets?month=YYYY-MM&currency=USD` returns exact monthly usage and
remaining limits. `PUT /v1/budgets` takes
`{id?, month, currency, category_id, amount_minor}`. Without ID it upserts a
monthly scope; ID permits moving/editing an owned budget. Scope conflicts
return 409. `DELETE /v1/budgets/{id}` removes a limit.
`POST /v1/budgets/copy` accepts `{from_month, to_month, currency}` and returns
`{added}`; it preserves destination budgets. Limits do not create postings.

`GET /v1/reports/summary` takes `currency` and the existing period selection
parameters. It returns opening/ending balances, income/spending/net,
`other_movement_minor`, category rows with parent IDs, previous-period totals
and exact changes (when applicable), plus six monthly trend points. It uses a
single repeatable-read snapshot. Parent category Activity filters now match
direct entries and subcategories. All routes require authentication and ownership
scoping. See BUDGETS_REPORTS_CATEGORIES.md for reconciliation and date semantics.

## Recurring schedules and cash-flow planning

`GET /v1/schedules` returns open schedules and recent ended schedules (up to
500), including wallet/category labels and the next pending date.
`POST /v1/schedules` and `PUT /v1/schedules/{id}` require `Idempotency-Key` and
`{account_id, currency, amount_minor, category_id?, note, cadence, first_due_date,
end_date?, revision?}`. Cadence is weekly/monthly/yearly. Edits require the
current revision and apply to pending dates from today; pattern changes require
a new first date on/after today.

`POST /v1/schedules/{id}/pause`, `/resume` or `/end` takes `{revision}`.
`POST /v1/schedules/sync` refreshes the owner's pending 90-day window without
posting money. The background worker uses the same idempotent operation.
Missing/foreign schedules return 404; stale/ended changes return 409; invalid
inputs or planning limits return 422.

`GET /v1/plans` additionally accepts `view=all|overdue|soon` and optional
`currency`. Soon is today through day 29; overdue is before today, in UTC.
Rows include `schedule_id` (zero for standalone plans). Canceling a recurring
plan skips that occurrence permanently without ending the schedule.

`POST /v1/plans/{id}/post` additionally accepts optional signed `amount_minor`
for the actual payment/receipt; omitted uses the estimate. The estimate is kept
and `recorded_amount_minor` is saved alongside its transaction. The date is
strictly YYYY-MM-DD and must respect the wallet starting date and today's UTC
date. A completed plan rejects changed actual details on replay.

`GET /v1/planning/cashflow` returns a coherent 30-day per-wallet estimate with
recorded balance, planned money in/out, projected/lowest day-end balances, overdue
counts and daily points. It includes standalone and recurring pending plans.
Overdue amounts are assumed to happen today; currencies are kept separate.
See RECURRING_BILLS_AND_CASHFLOW.md for semantics, controls and limits.
