# LedgerX

LedgerX is a self-hosted personal finance web app for tracking income, expenses,
wallets and budgets. It supports manual entry and CSV imports without requiring
a bank connection or paid API.

The current web app is maintained on the `codex/modern-ui` branch.

## Features

- Wallets with starting balances, income, expenses, refunds and transfers.
- Searchable transaction history, category filters and reviewed corrections.
- Income and expense categories, subcategories, and monthly budgets.
- Weekly, monthly, yearly, custom and all-time reports with CSV export.
- CSV import with column mapping, duplicate review and Money Lover history support.
- Planned transactions, weekly/monthly/yearly recurring schedules, and per-wallet
  cash-flow projections. Payments stay planned until explicitly confirmed.
- Category suggestions based on personal history and keywords, with manual acceptance.
- Financial workspace backup/restore, recovery codes and session management.

## Run locally

Requirements: Git, Bash, OpenSSL, and a running Docker engine with Compose v2.
On macOS, Docker Desktop provides Docker and Compose; OpenSSL is included.
Go and Node.js are not required to run the packaged app.

```sh
git clone --branch codex/modern-ui https://github.com/EftikharAzim/LedgerX.git
cd LedgerX
./scripts/app.sh start
```

Open **http://localhost:8088**. The first build downloads dependencies and takes
longer than subsequent starts. Startup builds the frontend and API, applies
database migrations, and starts PostgreSQL and Redis.

The script creates a private `.env.app` file and stores data in Docker volumes.
Keep both: stopping or rebuilding the app preserves records, but deleting its
volumes removes them. The packaged app uses a separate database from the
development setup.

| Command | Purpose |
| --- | --- |
| `./scripts/app.sh status` | Check service status. |
| `./scripts/app.sh logs` | Follow application logs. |
| `./scripts/app.sh stop` | Stop services while preserving data. |
| `./scripts/app.sh backup` | Back up the database, Redis and report files. |

### Update an existing installation

From the repository root on `codex/modern-ui`:

```sh
./scripts/app.sh backup
git pull --ff-only
./scripts/app.sh start
```

Server backups are written to `tmp/server-backups/` and contain credentials as
well as financial data. Keep them private and copy them off the host for recovery
from disk loss. See [running and deployment](docs/RUNNING_AND_DEPLOYMENT.md) for
restore instructions and configuration options.

## Development

Requirements: Go 1.26.10, Node.js 24, Docker and Compose v2.

From the repository root:

```sh
[ -f .env ] || cp .env.example .env
npm ci --prefix ledgerx-ui
make dev
```

In a second terminal, also at the repository root:

```sh
make ui-dev
```

The development UI runs at **http://localhost:5173** and proxies API requests to
port **8080**. Keep `VITE_API_BASE_URL` unset for this setup. The packaged app
uses port **8088** and does not need a Vite server.

See the [frontend guide](ledgerx-ui/README.md) for module organization and UI
conventions. SQLC queries are generated from `internal/repo/queries/`; run
`make gen` after changing query sources.

### Checks

```sh
go test ./... -race -count=1
make lint
npm --prefix ledgerx-ui test
npm --prefix ledgerx-ui run lint
npm --prefix ledgerx-ui run build
make audit
```

Integration tests run when `TEST_DATABASE_URL` is set. Use a disposable database
whose name contains `test` and a separate Redis instance via `TEST_REDIS_ADDR`.
The suite drops and recreates the test database's public schema; never point it
at application data. Without `TEST_DATABASE_URL`, the integration suite is skipped.

The [release checks](docs/RUNNING_AND_DEPLOYMENT.md#release-checks) cover browser
workflows, mobile layouts, accessibility, container replacement and server
recovery. Run them against an isolated stack containing synthetic records.

## Implementation

The Go process serves the API and built React frontend, and runs background
workers. PostgreSQL is the source of truth; Redis provides job queues, caches,
rate limiting and an event stream. The backend uses chi, pgx, SQLC and Asynq.
The frontend uses React, TypeScript, Vite and TanStack Query.

Money is stored in integer minor units. Postings must balance to zero, enforced
by service validation and a deferred PostgreSQL constraint. Ledger writes,
idempotency records and outbox events commit together. Corrections retain linked
history through reversal and replacement transactions.

Report periods use UTC. Currencies stay separate; exchange-rate conversion is
not inferred. Reports distinguish spending from transfers and starting balances,
and refunds reduce spending. Pending plans do not change recorded balances.

## Documentation

| Guide | Contents |
| --- | --- |
| [Architecture](docs/ARCHITECTURE.md) | Components, dependencies and process lifecycle. |
| [Data model](docs/DATA_MODEL.md) | Schema, postings and accounting invariants. |
| [API reference](docs/API.md) | Endpoints, authentication and request formats. |
| [Imports and balance checks](docs/IMPORTS_AND_BALANCE_CHECKS.md) | History imports, duplicates and reconciliation. |
| [Budgets and reports](docs/BUDGETS_REPORTS_CATEGORIES.md) | Category management, budgets and report behavior. |
| [Recurring plans](docs/RECURRING_BILLS_AND_CASHFLOW.md) | Schedules, payment confirmation and cash-flow estimates. |
| [Backup and recovery](docs/WORKSPACE_RECOVERY.md) | Portable financial backups and reviewed restores. |
| [Engineering review](docs/ENGINEERING_REVIEW.md) | Implemented fixes, verification and remaining work. |

The [documentation index](docs/README.md) also covers account security,
deployment, dependency audits and product planning.

## Current limitations

LedgerX is in active development. No public hosted service is configured.
Bank synchronization, automatic payment capture, shared wallets, debt/loan
management and native mobile apps are not implemented. Local validation does
not establish production capacity; see the
[launch and scaling assessment](docs/LAUNCH_AND_SCALING.md).

## License

MIT.
