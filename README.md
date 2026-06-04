# aurelion-backplane

Go backend for the Aurelion platform: API, persistence, messaging, and
capability engines. The HTTP API is one process (`cmd/backplane`);
several sibling processes (worker, ingester, pdp, inference gateway, log
bridges) share the same `internal/` packages and bootstrap.

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
├── worker/                       orchestrator runner (pipeline runs)
├── ingester/                     ingest MQ consumer → data lake
├── pdp/                          policy decision point (AuthN / AuthZ)
├── inference-gateway/            single network entry point for LLM inference
├── migrate/                      bun migration runner
├── log-siem-transmitter/         logs topic → SIEM bridge
└── log-dev-projector/            in-memory log viewer (dev only)
internal/
├── core/                         single-implementation infra
│   ├── config/                   Settings struct + Load (no I/O)
│   ├── cartridges/               cartridge provider (filesystem)
│   ├── correlation/              X-Correlation-ID context helpers
│   ├── descriptor/               cartridge descriptor surface
│   ├── events/                   domain Envelope + Sink + MQ publisher
│   ├── logger/                   slog Logger factory
│   ├── orchestrator/             pipeline catalog + beat + matcher
│   ├── pipelines/ policies/      cartridge-sourced catalogs
│   ├── postgres/ rabbitmq/       bun.DB and amqp factories
│   └── webserver/                echo.Echo factory + middleware
├── inventory/                    domain entities + posture state (L1)
│                                   persons, employments, org-units,
│                                   workloads, principals, accounts,
│                                   capability-grants, secrets, consent,
│                                   findings, evidence-chain,
│                                   policy-assessment-runs, …
├── engines/                      capability engines (L2)
│   ├── inventory_import/discover/ingest/normalize
│   ├── policy_assessment/        mechanism dispatch (cedar / opa / sod / …)
│   ├── risk/                     factor-decomposed priority scorer
│   ├── owner_assignment/         finding → accountable owner
│   ├── access_generate/          initiatives a principal should hold
│   ├── compliance_projection/    findings → external control languages
│   └── finding_explanation/      proven finding → cited narrative
├── integrations/                 governed systems + connectors
│   ├── applications/             Application entity + CRUD + matching
│   └── connectors/               ConnectorInstance registry + RPC
├── platform/                     switchable backends
│   ├── llm/                      Stream provider, protocol-keyed
│   │                               (openai-compatible / anthropic / gemini)
│   ├── secretmanagers/           Manager implementations (file + 4 stubs)
│   ├── siem/                     audit-log Sinks (file, mq, stdout + stubs)
│   └── storage/                  data-lake batch providers (file + 2 stubs)
├── transports/                   non-HTTP entry points
│   └── ingest_mq/                ingest-queue consumer wiring
└── migrations/                   bun migration registry (.go files)
```

## Dependency direction

```
cmd            → everything (composition root)
core/config    → platform/secretmanagers (Manager interface)
core/*         → nothing inside backplane (except config → secretmanagers)
inventory/*    → core/* (bun models, repositories, thin routes)
integrations/* → core/*, platform/* (via DI from main)
platform/*     → core/* (adapters reference the port they implement)
engines/*      → core/*, platform/*, inventory/*, integrations/* (when wired)
```

`platform/*` never imports `core/config` — adapters get raw inputs
(`postgres.Config{DSN, Debug}`, etc.) from the composition root. The
same rule applies to `core/*` packages: they receive every value they
need from `main.go`, never read env or call into config themselves.
`integrations/applications` does not import `integrations/connectors`
directly — `main.go` wires a small adapter that satisfies the
`MatchingProvider` interface.

`core/` vs `inventory/` vs `engines/` vs `platform/` vs `integrations/`:
- **core/** holds single in-process implementations we committed to:
  one Postgres driver, one HTTP server, one slog setup, one RabbitMQ
  client, the cartridge provider and pipeline orchestrator.
- **inventory/** holds the domain entities and posture state (L1) —
  identities, accounts, grants, secrets, consent, findings, evidence.
- **engines/** holds the capability engines (L2) that act over that
  state — assessment, projection, explanation, ingest, scoring.
- **platform/** holds anything where multiple backends are legitimate
  and one is picked by config (file vs mq vs Splunk for `siem`; file
  vs vault for `secretmanagers`; a protocol-keyed client for `llm`).
- **integrations/** holds the catalog of external systems Aurelion
  governs (Applications) and the runtime registry / transport for the
  backends those applications are served by (ConnectorInstances).
  Connector implementations themselves live OUTSIDE backplane — they
  self-register over MQ.

## Bootstrap flow

1. `cmd/backplane/main.go` calls `godotenv.Load()` (best-effort).
2. Reads `AURELION_SECRET_PROVIDER` from env.
3. Constructs a `secretmanagers.Manager` via `secretmanagers.Factory.Get(provider)`.
4. `config.Load(manager)` returns an immutable `*config.Settings`.
5. `logger.New`, `postgres.New`, `rabbitmq.New`, the cartridge provider,
   log sinks, capability engines, and `webserver.New` are wired in
   order. Each fails fast on unreachable dependencies.
6. HTTP server starts; the process blocks on SIGINT / SIGTERM and
   performs an orderly shutdown.

Every sibling binary (`worker`, `ingester`, `pdp`, `inference-gateway`)
boots the same way — same secret-store plumbing and config — then runs
its own loop instead of (or alongside) the HTTP API.

## Local run

```bash
cp .env.example .env
cp .secrets.example.json .secrets.json
make tidy
make migrate-up   # apply every unapplied migration
make run          # HTTP API only
# or: make run-all  # API + worker + ingester + pdp + inference-gateway + log bridges
```

Postgres and RabbitMQ are expected on `localhost:5432` and
`localhost:5672`. LLM inference additionally expects the inference
gateway (`cmd/inference-gateway`, default `:8090`); see
`internal/platform/llm/README.md` and the docs for runtime setup.

## Commands

```bash
make tidy             # go mod tidy
make fmt              # gofmt -s -w .
make vet              # go vet ./...
make build            # bin/{backplane,worker,ingester,pdp,inference-gateway,log-*,migrate}
make run              # go run ./cmd/backplane
make run-all          # every long-running binary in one terminal
make test             # go test ./...
make check            # fmt + vet + test
make migrate-init     # create the bun_migrations table
make migrate-up       # apply every unapplied migration
make migrate-down     # revert the last applied migration
make migrate-status   # print applied / pending sets
```

## HTTP surface (v0)

Mounted at `/api/v0` from the composition root. The table groups the
surface by area; per-entity fields and filters live in the docs
reference, not here.

| Area | Representative paths |
|---|---|
| Liveness | `GET /healthz` |
| Integrations | `…/applications` (CRUD + `…/{id}/matching-connector-instances`), `…/connector-instances` |
| Identity inventory | `…/persons`, `…/employments`, `…/employee-records`, `…/org-units`, `…/customers`, `…/principals`, `…/workloads`, `…/accounts`, `…/workloads/{id}/lineage` |
| Access & posture | `…/capability-grants`, `…/persons/{id}/access-profile`, `…/initiatives`, `…/findings`, `…/evidence-chains`, `…/policy-assessment-runs`, `…/policy-evaluation-outcomes` |
| Credentials & consent | `…/secrets/plain`, `…/secrets/certificates`, `…/consented-applications`, `…/consent-grants` |
| Compliance projection | `…/policy-assessment-runs/{id}/projections[/{projection}[/controls/{control}\|/packet]]` |
| Finding explanation | `POST …/findings/{id}/explanations`, `…/explanations/latest`, `…/explanation-jobs/{id}` |
| Ingest & catalogs | `…/inventory/import` · `…/inventory/discover` · `…/inventory/ingest`, `…/cartridges`, `…/descriptor`, pipeline & policy catalogs, orchestrator runs |

Inventory and posture surfaces are read-mostly over HTTP; writes flow
through connector/ingest actions and the assessment engines, not direct
REST mutation.

`X-Correlation-ID` is echoed (or generated) on every response and
propagated to events, log entries, and connector RPC calls so a single
trace id spans the whole call chain.
