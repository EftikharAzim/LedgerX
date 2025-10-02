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
