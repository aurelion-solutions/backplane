# aurelion-backplane

Single-process Go backend for the Aurelion platform: API, persistence,
messaging, and capability engines under one service.

## Stack

- Go 1.26
- echo/v4 — HTTP
- uptrace/bun + pgdriver — Postgres and typed query builder
- rabbitmq/amqp091-go — RabbitMQ
- log/slog — structured process logging (stdlib)
- joho/godotenv — bootstrap envvar loading

## Layout

```
cmd/backplane/main.go             composition root
internal/
├── core/                         single-implementation infra
│   ├── config/                   Settings struct + Load (no I/O)
│   ├── secret/                   Manager interface + Factory registry
│   ├── logger/                   slog Logger factory
│   ├── postgres/                 bun.DB factory
│   ├── rabbitmq/                 amqp Conn factory
│   └── webserver/                echo.Echo factory
├── platform/                     switchable backends
│   ├── logsink/                  business / audit log Sinks (file, mq, +9 stubs)
│   └── secretmanagers/           Manager implementations (file, +future vault/aws)
└── domain/                       domain packages — capability ports + types
migrations/                       golang-migrate destination
```

## Dependency direction

```
cmd          → everything (composition root)
core/config  → core/secret
core/*       → nothing inside backplane
platform/*   → core/secret (adapters reference the port they implement)
domain/*     → core/*, platform/*  (when written)
```

`platform/*` never imports `core/config` — adapters get raw inputs
(`postgres.Config{DSN, Debug}`, etc.) from the composition root. The
same rule applies to `core/*` packages: they receive every value they
need from `main.go`, never read env or call into config themselves.

`core/` vs `platform/`:
- **core/** holds the single in-process implementations we committed
  to: one Postgres driver, one HTTP server, one slog setup, one
  RabbitMQ client. No alternatives, no plugin surface.
- **platform/** holds anything where multiple backends are legitimate
  and one is picked by config (file vs mq vs Splunk vs Loki for
  `logsink`; file vs vault vs aws for `secretmanagers`).

## Bootstrap flow

1. `cmd/backplane/main.go` calls `godotenv.Load()` (best-effort).
2. Reads `AURELION_SECRET_PROVIDER` from env.
3. Constructs a `secret.Manager` via `secret.Factory.Get(provider)`.
4. `config.Load(manager)` returns an immutable `*config.Settings`.
5. `logger.New`, `postgres.New`, `rabbitmq.New`, `logsink` providers
   and `webserver.New` are wired in order. Each fails fast on
   unreachable dependencies.
6. HTTP server starts; the process blocks on SIGINT / SIGTERM and
   performs an orderly shutdown.

## Local run

```bash
cp .env.example .env
cp .secrets.example.json .secrets.json
make tidy
make run
```

Postgres and RabbitMQ are expected on `localhost:5432` and
`localhost:5672`.

## Commands

```bash
make tidy    # go mod tidy
make fmt     # gofmt -s -w .
make vet     # go vet ./...
make build   # → bin/backplane
make run     # go run ./cmd/backplane
make test    # go test ./...
make check   # fmt + vet + test
```
