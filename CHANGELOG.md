# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
this project adheres to [Semantic Versioning](https://semver.org/).

## [0.1.0] — 2026-03-13

Initial bootstrap skeleton of `aurelion-backplane`: a single-process Go
backend covering API, persistence, messaging, audit log delivery, and
orchestrator scaffolding.

### Added

#### Project

- Go 1.26 module `github.com/aurelion-solutions/backplane`.
- `LICENSE` (BUSL-1.1) and `CLA.md` carried over from the Aurelion
  trunk; SPDX headers on every Go file.
- `.env.example`, `.secrets.example.json`, `.gitignore`, `Makefile`,
  top-level `README.md` with layout and dependency-direction rules.
- File-based `.secrets.json` stores native JSON objects per key —
  hand-editable, no escaped strings.

#### Core layer (`internal/core/`)

- `config/` — `Settings` aggregate with one file per section
  (`postgres.go`, `rabbitmq.go`, `app.go`), pure value types and a
  central `loader.go` with `decodeRequired` / `decodeOptional` helpers.
- `secret/` — `Manager` (read-only), `Mutator` (write), and
  `FullManager` (union) contracts; `ErrNotFound`, `ErrNotImplemented`.
- `logger/` — `slog.JSONHandler` factory with string-level parsing.
- `postgres/` — `bun.DB` factory with pgdriver pool, fail-fast `Ping`.
- `rabbitmq/` — connection + channel factory, typed `Exchange{Name, Type}`
  declarations (`Topic` / `Direct` / `Fanout` / `Headers` constants);
  generic `Consume` helper that declares queue, binds keys, and
  dispatches deliveries to a callback with ack/nack semantics.
- `webserver/` — `echo.Echo` factory with recover, request-id, CORS
  middleware, slog access log, and `/healthz`.
- `events/` — domain `Envelope` schema with `<domain>.<entity>.<operation>`
  routing-key grammar, `ParticipantKind`, `NewEnvelope` / `NewRoot` /
  `NewDownstream` constructors with validation; `Sink` interface, MQ
  publisher, and `Tee` fan-out helper.

#### Platform layer (`internal/platform/`)

- `secretmanagers/` — `Factory` registry, `File` provider with
  live-read + atomic temp/rename writes, `Stub` base, and stubs for
  `Vault`, `OpenBao`, `Akeyless`, `Conjur`.
- `siem/` — structured audit-log `Event` with frozen schema,
  trace/correlation propagation, `Sink` / `Reader` contracts, `Factory`
  registry, `Stub` base, real `File` (JSONL append + mutex) and `MQ`
  (topic publish with routing key `<component>.<level>`) sinks,
  `Stdout` (JSON-per-line), `MultiSink` fan-out, `EmitInfo` lifecycle
  helper, plus stubs for `ELK`, `Fluentd`, `Loki`, `Nagios`, `QRadar`,
  `Rsyslog`, `Seq`, `Splunk`, `Zabbix`.
- `storage/` — data-lake batch contract (`WriteBatch` / `ReadBatch` /
  `DeleteBatch`), `Factory`, `Stub` base, `File` provider writing
  per-dataset JSONL batches with path-traversal validation, and stubs
  for `S3` and `Iceberg`.
- `llm/` — chat-streaming `Provider` interface (channel-based
  `Stream`), `Message`, `Chunk`, `Role`, `Factory`, `Stub` base, and
  stubs for `LlamaCpp` (on-prem GGUF), `Anthropic`, `OpenAI`.

#### Engines (`internal/engines/`)

- `orchestrator/` — skeleton with domain `types` (`Pipeline`, `Step`,
  `Run`, `RunStep`, `RunStatus`, `StepStatus`), ports (`Repository`,
  `Loader`, `Dispatcher`, `StepExecutor`), `Service` API with
  `ErrNotImplemented` stubs (`StartRun`, `GetRun`, `CancelRun`,
  `ReportStepResult`), and `Runner` heartbeat loop.

#### Binaries (`cmd/`)

- `backplane/` — composition root. Wires secrets → config → Postgres
  → RabbitMQ → events → storage → SIEM (multi-sink: file + stdout) →
  LLM → webserver. Retries Postgres and RabbitMQ connection in a loop
  on failure (5 s interval, cancellable). Emits lifecycle Events
  through MQ on start/stop.
- `worker/` — stand-alone orchestrator runner. Skeleton heartbeat loop;
  emits lifecycle Events through MQ on start/stop.
- `log-siem-transmitter/` — bridges the `aurelion.logs` topic
  exchange (queue `aurelion.logs.siem`, `#` binding) to the configured
  `siem.Sink`. Multi-sink ready (default: `file`-only — `stdout` is
  excluded on purpose since this consumer's terminal is not the
  publisher's). Includes `README.md` and start-time banner.
- `log-dev-projector/` — in-memory log viewer for local development.
  Consumes `aurelion.logs.buffer` queue into a fixed-size ring with
  FIFO eviction and serves `GET /buffer?limit=N` + `GET /healthz` on
  `:8001`. Includes `README.md` and start-time banner.

### Architectural rules established

- **Module layering**. `cmd/*` is the only composition root; `core/*`
  holds single-implementation infrastructure; `platform/*` holds
  switchable backends; `engines/*` holds capability engines.
- **Dependency direction**. `cmd → engines → platform → core`. Core
  packages never import `core/config`; the composition root assembles
  every `Config` value and hands it in.
- **Env-vars budget**. The only env reads in the whole service are
  `AURELION_SECRET_PROVIDER` and `AURELION_SECRETS_FILE`. Everything
  else lives in code constants or in the secret store.
- **Secret-vs-app split**. Secret store holds credentials for external
  systems; application settings live inside the application.
- **Port/adapter split for switchable infra**. The contract lives in
  `core/secret`; implementations live alongside the factory inside
  `platform/secretmanagers/`.
