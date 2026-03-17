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
cmd/
├── backplane/                    composition root + HTTP API
├── worker/                       orchestrator runner
├── log-siem-transmitter/         logs topic → SIEM bridge
├── log-dev-projector/            in-memory log viewer (dev only)
└── migrate/                      bun migration runner
internal/
├── core/                         single-implementation infra
│   ├── config/                   Settings struct + Load (no I/O)
│   ├── correlation/              X-Correlation-ID context helpers
│   ├── events/                   domain Envelope + Sink + MQ publisher
│   ├── logger/                   slog Logger factory
│   ├── postgres/                 bun.DB factory
│   ├── rabbitmq/                 amqp Conn + Consume + RPC client
│   ├── secret/                   Manager interface
│   └── webserver/                echo.Echo factory + middleware
├── integrations/                 catalog of governed systems + adapters
│   ├── applications/             Application entity + CRUD + matching
│   └── connectors/               ConnectorInstance registry + selector
│                                   + registration consumer + RPC client
├── platform/                     switchable backends
│   ├── llm/                      Stream provider (Anthropic / OpenAI / on-prem)
│   ├── secretmanagers/           Manager implementations (file + 4 stubs)
│   ├── siem/                     audit-log Sinks (file, mq, stdout + 9 stubs)
│   └── storage/                  data-lake batch providers (file + 2 stubs)
├── engines/                      capability engines
│   └── orchestrator/             pipeline runner skeleton
└── migrations/                   bun migration registry (.go files)
```

## Dependency direction

```
cmd            → everything (composition root)
core/config    → core/secret
core/*         → nothing inside backplane
integrations/* → core/*, platform/* (via DI from main)
platform/*     → core/* (adapters reference the port they implement)
engines/*      → core/*, platform/*, integrations/* (when wired)
```

`platform/*` never imports `core/config` — adapters get raw inputs
(`postgres.Config{DSN, Debug}`, etc.) from the composition root. The
same rule applies to `core/*` packages: they receive every value they
need from `main.go`, never read env or call into config themselves.
`integrations/applications` does not import `integrations/connectors`
directly — `main.go` wires a small adapter that satisfies the
`MatchingProvider` interface.

`core/` vs `platform/` vs `integrations/`:
- **core/** holds single in-process implementations we committed to:
  one Postgres driver, one HTTP server, one slog setup, one RabbitMQ
  client.
- **platform/** holds anything where multiple backends are legitimate
  and one is picked by config (file vs mq vs Splunk for `siem`; file
  vs vault for `secretmanagers`; file vs S3 vs Iceberg for `storage`).
- **integrations/** holds the catalog of external systems Aurelion
  governs (Applications) and the runtime registry / transport for the
  backends those applications are served by (ConnectorInstances).
  Connector implementations themselves live OUTSIDE backplane — they
  self-register over MQ.

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
make migrate-up   # creates applications + connector_instances
make run
```

Postgres and RabbitMQ are expected on `localhost:5432` and
`localhost:5672`.

## Commands

```bash
make tidy             # go mod tidy
make fmt              # gofmt -s -w .
make vet              # go vet ./...
make build            # bin/{backplane,worker,log-*,migrate}
make run              # go run ./cmd/backplane
make run-all          # every binary in one terminal
make test             # go test ./...
make check            # fmt + vet + test
make migrate-init     # create the bun_migrations table
make migrate-up       # apply every unapplied migration
make migrate-down     # revert the last applied migration
make migrate-status   # print applied / pending sets
```

## HTTP surface (v0)

| Method | Path | Purpose |
|---|---|---|
| GET | `/healthz` | liveness probe |
| GET | `/api/v0/applications` | list every Application |
| POST | `/api/v0/applications` | create an Application |
| PATCH | `/api/v0/applications/{id}` | partial update |
| DELETE | `/api/v0/applications/{id}` | delete |
| GET | `/api/v0/applications/{id}/matching-connector-instances?online_only=true` | online connectors that cover the application's required tags |
| GET | `/api/v0/connector-instances` | list every registered connector instance |
| GET | `/api/v0/connector-instances/{instance_id}` | single instance by external `instance_id` |

`X-Correlation-ID` is echoed (or generated) on every response and
propagated to events, log entries, and connector RPC calls so a
single trace id spans the whole call chain.
