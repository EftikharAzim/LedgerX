# LedgerX: A Double-Entry Accounting System

[![Go Version](https://img.shields.io/badge/Go-1.20+-blue.svg)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

LedgerX is a double-entry accounting system built with Go. Every transaction is a header plus two or more postings that must sum to zero — enforced in the service layer and again by a deferred database constraint trigger, so value can never be created or destroyed. Each user gets an `external` account representing the outside world, which absorbs the offsetting leg of income and expense entries. Inspired by the core functionality of systems like Stripe's balance engine, LedgerX prioritizes correctness and observability.

## Use Cases

LedgerX can serve as the backbone for a variety of applications:

- **Digital Wallets or Banking Apps:** Build a mobile or web application on top of this API to allow users to hold balances, send money, and view their transaction history.
- **Internal Business Ledgers:** Track internal finances, manage departmental budgets, or log payments to suppliers.
- **Fintech Product Backbones:** Power new financial products like "buy now, pay later" services, loyalty points systems, or peer-to-peer lending platforms.

## Key Features

- **Double-Entry Accounting**: Transactions are headers + postings with a zero-sum invariant enforced at two layers (service validation and a deferred Postgres constraint trigger). Transfers between accounts are first-class, and the ledger is append-only — mistakes are corrected with linked reversal transactions, never edits.
- **Idempotent Transaction Processing**: The idempotency-key claim, ledger write, outbox event, and cached response commit in a single database transaction — a crash or retry at any point cannot duplicate a transaction.
- **Transactional Outbox**: Events are written atomically with the ledger change and published by a worker that claims batches with `FOR UPDATE SKIP LOCKED` inside a transaction (at-least-once delivery).
- **User & Auth Management**: JWT auth (HS256-pinned, required signing key outside dev), bcrypt passwords, per-user rate limiting on the API plus per-IP limits on login/register (fail-closed when Redis is down), ownership checks on every account-scoped endpoint.
- **Asynchronous Task Processing**: Background workers (Asynq) for CSV exports and nightly balance snapshots; balances derive from snapshot + delta cut on insertion time, so backdated entries can't corrupt them. Exports go to pluggable storage — local disk in dev, S3/MinIO in deployments.
- **RESTful API**: Cursor-paginated transaction history, transfers, summaries, and authenticated exports.
- **Observability**: Structured logging and Prometheus metrics for monitoring system health.
- **Containerized Deployment**: Comes with Docker and Docker Compose for easy setup and deployment.

## Technology Stack

- **Backend**: Go
- **Database**: PostgreSQL
- **In-Memory Store**: Redis (for background job queuing)
- **API Framework**: Chi (v5)
- **Background Jobs**: Asynq
- **Database Migrations**: golang-migrate
- **SQL Code Generation**: sqlc
- **Containerization**: Docker, Docker Compose

## Architecture

The diagram below illustrates the high-level architecture of the LedgerX system, showing how the API, background workers, and data stores interact.

See **[docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md)** for the system context, component diagram, and request lifecycle; **[docs/DATA_MODEL.md](./docs/DATA_MODEL.md)** for the entity-relationship diagram and the double-entry invariants; and **[docs/SEQUENCES.md](./docs/SEQUENCES.md)** for sequence diagrams of the key flows (transaction creation, transfers, reversals, balance computation, async export, and the outbox).

## API Reference

Here is a summary of the main API endpoints. See [docs/API.md](./docs/API.md) for full details. All `/v1/*` routes require `Authorization: Bearer <JWT>`.

| Method | Endpoint                              | Description                                          |
| :----- | :------------------------------------ | :--------------------------------------------------- |
| `POST` | `/auth/register`                      | Register a new user (returns a JWT).                 |
| `POST` | `/auth/login`                         | Log in and receive a JWT.                            |
| `GET`  | `/v1/users/me`                        | Get the current user's profile.                      |
| `POST` | `/v1/accounts`                        | Create a new financial account.                      |
| `GET`  | `/v1/accounts`                        | List your accounts.                                  |
| `GET`  | `/v1/accounts/{id}/balance`           | Current balance (snapshot + delta).                  |
| `GET`  | `/v1/accounts/{id}/transactions`      | Cursor-paginated ledger entries, newest first.       |
| `GET`  | `/v1/accounts/{id}/summary?month=`    | Monthly inflow/outflow/net (cached).                 |
| `POST` | `/v1/transactions`                    | Record income/expense (idempotency-key support).     |
| `POST` | `/v1/transactions/{id}/reverse`       | Correct a transaction with a linked reversal.        |
| `POST` | `/v1/transfers`                       | Move money between two of your accounts.             |
| `POST` | `/v1/exports?month=`                  | Request a CSV export of a month's transactions.      |
| `GET`  | `/v1/exports/{id}/status`             | Export status (owner only).                          |
| `GET`  | `/v1/exports/{id}/download`           | Download the finished CSV (owner only).              |

## Project Structure

The project follows a clean architecture, separating concerns into distinct layers:

```
cmd/ledgerx/           # Main application entrypoint (wiring, graceful shutdown)
internal/
  observability/       # Structured logging, Prometheus metrics, pgx tracer
  repo/                # pgx pool + sqlc-generated queries (internal/repo/sqlc)
  service/             # Business logic: transactions, balance, summary, auth
  storage/             # Pluggable export storage (local disk / S3)
  transport/http/      # HTTP handlers, middleware, routing
  worker/              # Background jobs: CSV export, snapshots, outbox publisher
  integration/         # Integration tests (real Postgres + Redis)
migrations/            # golang-migrate SQL migrations
deploy/                # Dockerfile and docker-compose
docs/                  # API reference and design docs (architecture, data model, sequences)
ledgerx-ui/            # React + Vite frontend
```

## Getting Started

This project includes VS Code tasks for common operations. If you are using VS Code, it is the recommended way to get started.

### Prerequisites

- Go (version 1.20 or newer)
- Docker and Docker Compose
- `golang-migrate` CLI (if not using VS Code tasks)
- `sqlc` CLI (if not using VS Code tasks)

### Installation & Running the Application

1.  **Clone the repository:**

    ```sh
    git clone https://github.com/EftikharAzim/LedgerX.git
    cd LedgerX
    ```

2.  **Set up environment variables:**
    Create a `.env` file from the example. The default values are configured to work with the provided Docker setup.

    ```sh
    cp .env.example .env
    ```

3.  **Start Services & Run the App:**

    - **With VS Code (Recommended):**

      1.  Run the `Dev: Up (DB+Redis)` task to start the database and Redis.
      2.  Run the `DB: Migrate Up` task to apply database migrations.
      3.  Run the `sqlc: Generate` task to generate Go code from your SQL queries.
      4.  Launch the application by running `go run ./cmd/ledgerx`.

    - **Without VS Code:**
      1.  Start services: `cd deploy && docker compose up -d postgres redis`
      2.  Run migrations: `export $(grep -v '^#' .env | xargs) && migrate -path migrations -database "$DATABASE_URL" up`
      3.  Generate code: `sqlc generate`
      4.  Run the app: `go run ./cmd/ledgerx`

    - **One command (Make target):**

      ```sh
      make dev
      ```
      This starts Postgres + Redis, waits for the DB, applies migrations, and runs the API.

    - **End-to-end smoke test:**

      ```sh
      make e2e-smoke
      ```
      This brings up dependencies, applies migrations, starts the API in the background, runs the stdlib smoke test, then stops the API.

    The server will start on the port specified in your `.env` file (default is `8080`).

### Frontend (Vite + React)

The UI lives in `ledgerx-ui` and talks to the API via `VITE_API_BASE_URL`.

1. Configure API access for the UI:

   - Option A — CORS (default):

     Keep `ledgerx-ui/.env` with:

     ```env
     VITE_API_BASE_URL=http://localhost:8080
     ```

     The API enables CORS for local dev.

   - Option B — Vite proxy (no CORS):

     Remove or comment `VITE_API_BASE_URL` so the UI uses relative paths. The Vite config proxies `/auth`, `/v1`, and `/exports` to `http://localhost:8080` during `npm run dev`.

2. Install deps and run the dev server:

   ```sh
   cd ledgerx-ui
   npm install
   npm run dev
   ```

   Or, via Make:

   ```sh
   make ui-dev
   ```

3. Build for production:

   ```sh
   cd ledgerx-ui
   npm run build
   ```

Notes:

- The API includes a minimal CORS middleware for dev.
- A Vite dev proxy is also configured; omit `VITE_API_BASE_URL` to use it.

## Development

### Tests

Unit tests cover the double-entry invariants (zero-sum legs, idempotency
request hashing), JWT/password auth, and cursor pagination encoding:

```sh
go test ./...
```

Integration tests run the real stack — the zero-sum trigger, concurrent
idempotent creation, transfers, reversals, backdated-entry balance integrity,
the transactional outbox, and summary cache invalidation — against Postgres
and Redis (CI runs them automatically via service containers):

```sh
docker exec deploy-postgres-1 psql -U ledgerx -d ledgerx -c "CREATE DATABASE ledgerx_test" # once
TEST_DATABASE_URL=postgres://ledgerx:ledgerx@localhost:5432/ledgerx_test?sslmode=disable \
TEST_REDIS_ADDR=localhost:6379 go test ./internal/integration/ -count=1
```

The end-to-end smoke test (`make e2e-smoke`) exercises register → accounts →
idempotent transaction replay → transfer → balances → history → reversal →
export (request, poll, authenticated download).

### Linting

To lint the codebase, run:

```sh
golangci-lint run
```

## License

MIT
