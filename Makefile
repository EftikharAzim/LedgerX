SHELL := /bin/bash
include .env

run:
	set -a; source .env; set +a; go run ./cmd/ledgerx

.PHONY: up down gen migrate-up migrate-down lint test

up:
	cd deploy && docker compose up -d postgres redis

down:
	cd deploy && docker compose down

gen:
	sqlc generate

migrate-up:
	migrate -path migrations -database "$(DATABASE_URL_LOCAL)" -verbose up

migrate-down:
	migrate -path migrations -database "$(DATABASE_URL_LOCAL)" -verbose down 1

lint:
	golangci-lint run

test:
	go test ./... -race -count=1

build:
	go build -o main ./cmd/ledgerx

smoke:
	python3 scripts/smoke_test_stdlib.py

# --- Frontend ---
.PHONY: ui-dev ui-build
ui-dev:
	cd ledgerx-ui && npm run dev

ui-build:
	cd ledgerx-ui && npm run build

# --- Dev Orchestration ---
.PHONY: dev
dev:
	@echo "Starting dependencies (Postgres, Redis)"
	$(MAKE) up
	@echo "Waiting for database on localhost:5432 ..."
	@for i in {1..30}; do \
		(echo >/dev/tcp/127.0.0.1/5432) >/dev/null 2>&1 && break; \
		sleep 1; \
	done
	@echo "Applying migrations"
	$(MAKE) migrate-up
	@echo "Starting API"
	$(MAKE) run

.PHONY: api-start api-stop api-wait e2e-smoke api-logs
api-start:
	mkdir -p tmp
	@echo "Starting API in background"
	@(set -a; source .env; set +a; nohup go run ./cmd/ledgerx > tmp/api.log 2>&1 & echo $$! > tmp/api.pid)
	$(MAKE) api-wait

api-stop:
	@if [ -f tmp/api.pid ]; then \
		PID=$$(cat tmp/api.pid); \
		if ps -p $$PID >/dev/null 2>&1; then kill $$PID; fi; \
		rm -f tmp/api.pid; \
		echo "API stopped"; \
	else echo "No API pid file"; fi

api-wait:
	@echo "Waiting for API on http://127.0.0.1:8080/healthz ..."
	@for i in {1..30}; do \
		curl -fsS http://127.0.0.1:8080/healthz >/dev/null 2>&1 && break; \
		sleep 1; \
	done

api-logs:
	@tail -n +1 -f tmp/api.log

e2e-smoke:
	$(MAKE) up
	@echo "Waiting for database on localhost:5432 ..."
	@for i in {1..30}; do \
		(echo >/dev/tcp/127.0.0.1/5432) >/dev/null 2>&1 && break; \
		sleep 1; \
	done
	$(MAKE) migrate-up
	$(MAKE) api-start
	-$(MAKE) smoke || (echo "Smoke test failed"; exit 1)
	$(MAKE) api-stop
