# syntax=docker/dockerfile:1
FROM node:24-bookworm-slim AS ui
WORKDIR /src
COPY ledgerx-ui/package.json ledgerx-ui/package-lock.json ./
RUN --mount=type=secret,id=proxy_ca --mount=type=cache,target=/root/.npm \
    if [ -f /run/secrets/proxy_ca ]; then export NODE_EXTRA_CA_CERTS=/run/secrets/proxy_ca; fi; \
    npm ci --strict-ssl=true --no-audit
COPY ledgerx-ui/ ./
# Always use the same origin; local frontend .env files must never enter a build.
RUN VITE_API_BASE_URL= npm run build

FROM golang:1.26.10 AS build
WORKDIR /src
ENV GOMAXPROCS=4
COPY go.mod go.sum ./
RUN --mount=type=secret,id=proxy_ca \
    if [ -f /run/secrets/proxy_ca ]; then export SSL_CERT_FILE=/run/secrets/proxy_ca; fi; \
    go mod download
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/ledgerx ./cmd/ledgerx \
    && CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/healthcheck ./cmd/healthcheck \
    && mkdir -p /out/exports

# One process serves the built UI and API; export files live on a named volume.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/ledgerx /app/ledgerx
COPY --from=build /out/healthcheck /app/healthcheck
COPY --from=build --chown=65532:65532 /out/exports /data/exports
COPY --from=ui /src/dist /app/ui
ENV UI_DIR=/app/ui
EXPOSE 8080
ENTRYPOINT ["/app/ledgerx"]
