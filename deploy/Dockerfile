# Build stage
FROM golang:1.25 AS build
WORKDIR /src
COPY . .
RUN go build -o /out/ledgerx ./cmd/ledgerx

# Runtime stage
FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=build /out/ledgerx /app/ledgerx
ENTRYPOINT ["/app/ledgerx"]
