# LedgerX

LedgerX is a financial ledger application designed to manage accounts, transactions, balance snapshots, exports, and user authentication. It is built with Go and uses PostgreSQL and Redis for data storage and caching.

## Features

- Account and user management
- Transaction recording and categorization
- Balance snapshots
- CSV export functionality
- Outbox pattern for reliable event delivery
- RESTful HTTP API
- Authentication and rate limiting
- Background workers for scheduled tasks
- Observability with logging and metrics

## Project Structure

```
cmd/ledgerx/           # Main application entrypoint
internal/
  domain/              # Core domain logic and observability
  repo/                # Database access and SQL queries
  service/             # Business logic services
  transport/http/      # HTTP API handlers and middleware
  worker/              # Background job workers
migrations/            # Database migration scripts
deploy/                # Docker and deployment files
tmp/exports/           # Temporary export files
```

## Getting Started

### Prerequisites

- Go 1.20+
- Docker & Docker Compose
- PostgreSQL
- Redis

### Setup

1. Clone the repository:
   ```sh
   git clone <repo-url>
   cd LedgerX
   ```
2. Copy `.env.example` to `.env` and update environment variables as needed.
3. Start dependencies:
   ```sh
   cd deploy && docker compose up -d postgres redis
   ```
4. Run database migrations:
   ```sh
   export $(grep -v '^#' .env | xargs) && migrate -path migrations -database "$DATABASE_URL" up
   ```
5. Generate Go code from SQL queries:
   ```sh
   sqlc generate
   ```
6. Build and run the application:
   ```sh
   go run cmd/ledgerx/main.go
   ```

## Development

- Run tests:
  ```sh
  go test ./... -race
  ```
- Lint code:
  ```sh
  golangci-lint run
  ```
- Manage database migrations:
  ```sh
  export $(grep -v '^#' .env | xargs) && migrate -path migrations -database "$DATABASE_URL" up
  ```

## API

See `internal/transport/http/` for available endpoints and request/response formats.

## License

MIT
