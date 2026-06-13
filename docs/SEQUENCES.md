# LedgerX — Sequence & Flow Diagrams

How the key flows actually execute at runtime. For the static structure see
[ARCHITECTURE.md](./ARCHITECTURE.md); for the schema and invariants see
[DATA_MODEL.md](./DATA_MODEL.md).

Participants are abbreviated: **H** = HTTP handler, **Svc** =
`TransactionService`, **PG** = PostgreSQL (inside one DB transaction unless
noted), **Redis**, **Worker** = Asynq worker, **Store** = export storage.

---

## 1. Create transaction (idempotent, with outbox)

The single most important flow. Everything from the idempotency claim to the
outbox event commits in **one DB transaction**, so the system can never end up
with a transaction but no event, or a charged key but no transaction.

```mermaid
sequenceDiagram
    autonumber
    actor C as Client
    participant H as Handler
    participant Svc as TransactionService
    participant PG as PostgreSQL (1 tx)
    participant R as Redis (cache)

    C->>H: POST /v1/transactions (+ Idempotency-Key?)
    H->>H: auth → uid, decode, validate fields
    H->>Svc: Create(uid, account, amount, currency, ...)
    Svc->>Svc: validateLegs (zero-sum / non-zero)
    Svc->>PG: BEGIN
    opt Idempotency-Key present
        Svc->>PG: INSERT idempotency_keys (ON CONFLICT DO NOTHING)
        alt key already existed
            Svc->>PG: SELECT existing record
            Note over Svc: replay → return cached JSON<br/>conflict → 409<br/>in-flight → 409
        end
    end
    Svc->>PG: resolve external account (offsetting leg)
    Svc->>PG: ownership + active + currency checks on every leg
    Svc->>PG: INSERT transactions (header)
    Svc->>PG: INSERT postings (account leg + external leg)
    Svc->>PG: INSERT outbox (TransactionCreated)
    opt Idempotency-Key present
        Svc->>PG: UPDATE idempotency_keys SET status=succeeded, response_json
    end
    Svc->>PG: COMMIT
    Note over PG: deferred trigger checks Σ postings = 0 here
    Svc->>R: invalidate summary cache (best effort, post-commit)
    Svc-->>H: TransactionDTO
    H-->>C: 201 Created
```

## 2. Idempotency decision logic

What happens when the key is claimed (step "key already existed" above),
broken out because the branching is the subtle part.

```mermaid
flowchart TD
    A["INSERT idempotency_keys<br/>ON CONFLICT DO NOTHING"] --> B{"row returned?"}
    B -->|yes, freshly claimed| C["proceed to write transaction"]
    B -->|no, key exists| D["SELECT existing record"]
    D --> E{"request_hash matches?"}
    E -->|no| F["409 — key reused<br/>with different payload"]
    E -->|yes| G{"status == succeeded<br/>& response cached?"}
    G -->|yes| H["return cached response<br/>(true replay)"]
    G -->|no| I["409 — request still in flight"]
```

## 3. Transfer between accounts

Same engine as create, but the legs are supplied explicitly and already sum to
zero, so **no external account is involved**. Ownership and matching currency
are checked on both legs.

```mermaid
sequenceDiagram
    autonumber
    actor C as Client
    participant H as Handler
    participant Svc as TransactionService
    participant PG as PostgreSQL (1 tx)

    C->>H: POST /v1/transfers (from, to, amount > 0, currency)
    H->>Svc: Transfer(...)
    Svc->>Svc: legs = [{from, -amount}, {to, +amount}] → validateLegs
    Svc->>PG: BEGIN
    Svc->>PG: (idempotency claim if key present)
    Svc->>PG: check both accounts: owned, active, currency matches
    Svc->>PG: INSERT header + 2 postings + outbox event
    Svc->>PG: COMMIT (deferred zero-sum trigger passes)
    Svc-->>H: TransactionDTO
    H-->>C: 201 Created
```

## 4. Reversal (append-only correction)

```mermaid
sequenceDiagram
    autonumber
    actor C as Client
    participant H as Handler
    participant Svc as TransactionService
    participant PG as PostgreSQL (1 tx)

    C->>H: POST /v1/transactions/{id}/reverse
    H->>Svc: Reverse(uid, id)
    Svc->>PG: BEGIN
    Svc->>PG: SELECT original WHERE id AND user_id
    alt not found / not owned
        Svc-->>H: 404
    else original is itself a reversal
        Svc-->>H: 409 (cannot reverse a reversal)
    end
    Svc->>PG: load original postings
    Svc->>PG: INSERT header (reversal_of = id)
    Note over PG: UNIQUE(reversal_of) → 409 if already reversed
    Svc->>PG: INSERT negated postings (-amount each)
    Svc->>PG: INSERT outbox (TransactionReversed)
    Svc->>PG: COMMIT (postings still sum to 0)
    Svc-->>H: 201 Created (reversal_of set)
```

## 5. Balance read (snapshot + delta)

A read path, no DB transaction needed. See the formula in
[DATA_MODEL.md](./DATA_MODEL.md#balances-snapshot--delta-cut-on-created_at).

```mermaid
sequenceDiagram
    autonumber
    actor C as Client
    participant H as Handler
    participant Bal as BalanceService
    participant PG as PostgreSQL

    C->>H: GET /v1/accounts/{id}/balance
    H->>PG: ownership check (account belongs to uid)
    H->>Bal: CurrentBalance(accountId, now)
    Bal->>PG: GetLatestSnapshot(account)
    Bal->>PG: SumPostingsSince(account, snapshot cutoff by created_at)
    Bal-->>H: snapshot.balance + delta
    H-->>C: 200 { balance_minor }
```

## 6. Asynchronous CSV export

Request and generation are decoupled through the Asynq queue; the client polls
status and downloads through the authenticated API (so the file's location —
disk or S3 — is invisible to the client).

```mermaid
sequenceDiagram
    autonumber
    actor C as Client
    participant H as Handler
    participant PG as PostgreSQL
    participant R as Redis (queue)
    participant W as Worker
    participant S as Store (disk/S3)

    C->>H: POST /v1/exports?month=YYYY-MM
    H->>PG: INSERT exports (status=pending)
    H->>R: enqueue export:csv job
    H-->>C: 202 Accepted { id, status: pending }

    R->>W: deliver job
    W->>PG: ListPostingsForMonth(user, month)
    W->>W: build CSV in memory
    W->>S: Save(export_<id>.csv)
    W->>PG: UPDATE exports SET status=done, file_path

    loop poll until done
        C->>H: GET /v1/exports/{id}/status
        H->>PG: GetExportByID (ownership checked)
        H-->>C: { status }
    end

    C->>H: GET /v1/exports/{id}/download
    H->>S: Open(file_path)
    H-->>C: 200 text/csv (streamed)
```

### Export status state machine

```mermaid
stateDiagram-v2
    [*] --> pending: POST /v1/exports
    pending --> done: worker wrote CSV
    pending --> error: query/write failed
    done --> [*]: client downloads
    error --> [*]
```

## 7. Outbox publisher

Runs every 5 seconds. Claims a batch **inside a transaction** so
`FOR UPDATE SKIP LOCKED` holds the locks until commit — concurrent publishers
get disjoint batches. Delivery is at-least-once (a crash after publish but
before commit re-publishes).

```mermaid
sequenceDiagram
    autonumber
    participant OB as Outbox publisher
    participant PG as PostgreSQL (1 tx)
    participant R as Redis (pub/sub)

    loop every 5s
        OB->>PG: BEGIN
        OB->>PG: SELECT ... WHERE processed_at IS NULL<br/>ORDER BY created_at LIMIT 50<br/>FOR UPDATE SKIP LOCKED
        loop each event
            OB->>R: PUBLISH events:<type> payload
        end
        OB->>PG: UPDATE outbox SET processed_at = now()<br/>WHERE id = ANY(published)
        OB->>PG: COMMIT
    end
```

## 8. Authentication

```mermaid
sequenceDiagram
    autonumber
    actor C as Client
    participant H as Auth handler
    participant PG as PostgreSQL

    rect rgb(245,245,245)
    note right of C: Register
    C->>H: POST /auth/register {email, password}
    H->>H: validate (email, password ≥ 8), bcrypt hash
    H->>PG: INSERT users (unique email)
    H-->>C: 201 { token } (HS256 JWT, 24h)
    end

    rect rgb(245,245,245)
    note right of C: Login + authenticated call
    C->>H: POST /auth/login {email, password}
    H->>PG: GetUserByEmail
    H->>H: bcrypt compare
    H-->>C: 200 { token }
    C->>H: GET /v1/... (Authorization: Bearer token)
    H->>H: ParseJWT (HS256 pinned, exp required) → uid in context
    end
```

`/auth/*` is rate-limited **per IP** (anti credential-stuffing); `/v1/*` is
rate-limited **per user**. Both fail **closed** (HTTP 503) if the limiter's
Redis is unreachable, so an outage can never silently disable the limit.
