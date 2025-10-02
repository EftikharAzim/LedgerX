# LedgerX

LedgerX is a robust, backend financial ledger engine built with Go, designed to provide a reliable and scalable foundation for any application that needs to manage user accounts and track monetary transactions. Inspired by the core functionality of systems like Stripe's balance engine, LedgerX prioritizes correctness, idempotency, and observability to ensure financial data is handled with the highest degree of integrity.

## Use Cases

LedgerX can serve as the backbone for a variety of applications:

- **Digital Wallets or Banking Apps:** Build a mobile or web application on top of this API to allow users to hold balances, send money, and view their transaction history.
- **Internal Business Ledgers:** Track internal finances, manage departmental budgets, or log payments to suppliers.
- **Fintech Product Backbones:** Power new financial products like "buy now, pay later" services, loyalty points systems, or peer-to-peer lending platforms.

## Features

- **Account & User Management:** Full CRUD operations for users and their financial accounts.
- **Idempotent Transactions:** Safely retry API requests without risk of creating duplicate transactions.
- **Transaction Categorization:** Organize transactions for better financial tracking.
- **Balance Snapshots:** Background workers periodically capture account balances for historical reporting.
- **Reliable Event Delivery:** Implements the outbox pattern to ensure events are delivered reliably.
- **RESTful HTTP API:** A clean, well-structured API for client interaction.
- **Authentication & Rate Limiting:** Secure endpoints with JWT-based auth and protect against abuse with rate limiting.
- **Observability:** In-depth logging and Prometheus metrics for monitoring system health.

## How a Client Interacts with LedgerX

A client application (e.g., a web frontend or mobile app) would use LedgerX by making HTTP requests to its API. A typical workflow includes:

1.  **Authentication:** The client authenticates a user via the `/auth/login` endpoint to get a JWT. This token is then sent with all future requests.
2.  **Account Management:** The client creates and manages user accounts through the `/accounts` endpoints.
3.  **Making Transactions:** To move funds, the client sends a `POST` request to `/transactions`, including an idempotency key to prevent duplicate charges.
4.  **Viewing Financials:** The client fetches transaction history, balances, and summaries from the `/transactions`, `/balance`, and `/summary` endpoints.
5.  **Data Exports:** The client can request a CSV export of transactions via the `/exports` endpoint, which is processed asynchronously by a background worker.

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

1.  **Clone the repository:**
    ```sh
    git clone https://github.com/EftikharAzim/LedgerX.git
    cd LedgerX
    ```
2.  **Configure Environment:** Copy `.env.example` to `.env` and update the variables for your local setup.
3.  **Start Dependencies:**
    ```sh
    cd deploy && docker compose up -d postgres redis
    ```
4.  **Run Database Migrations:**
    ```sh
    export $(grep -v '^#' .env | xargs) && migrate -path migrations -database "$DATABASE_URL" up
    ```
5.  **Generate Go Code from SQL:**
    ```sh
    sqlc generate
    ```
6.  **Run the Application:**
    ```sh
    go run cmd/ledgerx/main.go
    ```

## Development

- **Lint Code:**
  ```sh
  golangci-lint run
  ```

## License

MIT
