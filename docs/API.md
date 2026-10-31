# LedgerX API Reference

All amounts are integers in minor units (cents). All timestamps are RFC 3339.

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

`/auth/*` is rate-limited per client IP.

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
| POST | `/v1/accounts` | `{name, currency}` | `201`; name unique per user |
| GET | `/v1/accounts` | `?limit=&offset=` | Includes the `external` account (`kind` field) |
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

## Transactions (double-entry)

Every transaction is a header plus ≥2 postings that sum to zero — enforced
both in the service layer and by a deferred database trigger.

| Method | Endpoint | Body | Notes |
| --- | --- | --- | --- |
| POST | `/v1/transactions` | `{account_id, amount_minor, currency, occurred_at, note?, category_id?}` | Income (+) / expense (−) against one account; the external account takes the offsetting leg. `201` with header + postings |
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

`/auth/*` is rate-limited per client IP; `/v1/*` per user. Limits surface in
`X-RateLimit-*` headers; when the limiter's Redis is unavailable requests fail
closed with `503`.

Validation failures return `422` (unbalanced, zero amount, inactive account,
currency mismatch), unknown/foreign accounts return `404`.

## Categories

| Method | Endpoint | Body | Notes |
| --- | --- | --- | --- |
| POST | `/v1/categories` | `{name}` | Create a category; unique per user (case-insensitive). `409` on duplicate |
| GET | `/v1/categories` | | List your categories |

New income/expense transactions that carry a note are asynchronously assigned a
suggested category by an LLM (see [AI_CATEGORIZATION.md](./AI_CATEGORIZATION.md)).
The suggestion appears on entries as `suggested_category_id` + `suggestion_score`;
confirm it via `POST /v1/transactions/{id}/category`. The AI never sets the
confirmed `category_id` directly.

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

## Operational

| Method | Endpoint | Notes |
| --- | --- | --- |
| GET | `/healthz` | Liveness |
| GET | `/readyz` | Checks Postgres + Redis |
| GET | `/metrics` | Prometheus |
| POST | `/admin/snapshot/run` | Requires `X-Admin-Token` matching `ADMIN_TOKEN`; disabled when unset |
