# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
this project adheres to [Semantic Versioning](https://semver.org/).

## [0.2.0] — 2026-03-17

### Added

#### Integrations layer (`internal/integrations/`)

- New `integrations/` layer between `platform/` and `engines/` for the
  catalog of governed systems and the runtime registry of adapters that
  serve them.
- `integrations/applications/` — `Application` domain entity (bun model
  on Postgres) with CRUD service, partial-patch payloads, REST handlers
  under `/api/v0/applications`, decommission flow that emits
  `inventory.application.decommissioned`, and a matching helper that
  surfaces eligible connectors for an application's required tag set.
- `integrations/connectors/` — `ConnectorInstance` registry,
  `CapabilityDescriptor` (operations, status transitions, supported
  fact kinds, account-disable cascades), pure tag-set `Selector`,
  `RegistrationConsumer` (durable topic consumer for self-registration
  and heartbeat messages), and REST handlers under
  `/api/v0/connector-instances`. Online window: 2 minutes; stale
  cutoff: 24 hours.
- Connector-specific `RPCClient` over the generic AMQP request/reply
  primitive: encodes the connector command shape (correlation_id,
  reply_exchange / reply_routing_key, operation, payload, optional
  trace fields, `result_storage_requested`); decodes the response
  envelope; handles non-ok status as typed `*ErrRPCStatus`; pulls large
  results out of the data lake via a `LakeReader` adapter when the
  remote peer returns `result_storage_ref`.

#### Core additions (`internal/core/`)

- `core/correlation/` — `X-Correlation-ID` carrier on `context.Context`
  with `WithID` / `ID` / `Ensure` helpers, mirroring the kernel
  `ContextVar` contract so service code can stamp events / log entries
  / RPC calls with one trace id.
- `core/webserver/` — `X-Correlation-ID` HTTP middleware: echoes the
  header when present, generates a fresh UUID v4 otherwise, attaches
  to the request context, propagates into slog access logs.
- `core/rabbitmq/rpc_client.go` — generic AMQP request/reply primitive.
  Opens its own dedicated channel on the shared `*amqp.Connection`,
  declares the responses exchange and a private exclusive auto-delete
  reply queue, correlates outgoing publishes with incoming replies by
  `correlation_id`, surfaces explicit timeouts (default 60 s), and
  exposes `ReplyTarget()` so protocol wrappers can encode the reply
  target into the command body when the wire shape requires it.

#### Migration tooling

- `internal/migrations/` — central bun migration registry
  (`migrations.Migrations`). Schemas land as raw SQL inside Go
  migration files so future model edits do not retroactively change
  historical migrations.
- Initial migrations: `applications` and `connector_instances` tables
  with the same column shape, indexes, and unique constraints as the
  kernel originals.
- `cmd/migrate/` — stand-alone runner with `init` / `up` / `down` /
  `status` commands. Reuses the same secret store as backplane.
- Makefile targets: `migrate-init`, `migrate-up`, `migrate-down`,
  `migrate-status`.

### Changed

- `cmd/backplane/main.go` — composition root now wires the
  integrations layer end-to-end: applications + connectors
  repositories, the connector RPC client, the registration consumer
  goroutine, the `/api/v0` router group, and the cross-slice
  `applications.MatchingProvider` adapter.
- `internal/core/config/rabbitmq.go` — adds
  `connector_registration_exchange` (default `aurelion.connectors.registry`)
  and `connector_registration_queue` (default `aurelion.connectors.registration`).
- `internal/core/webserver/webserver.go` — installs the
  correlation-id middleware and threads `correlation_id` into every
  access-log line.

### Fixed

- `integrations/applications/Repository.List` and
  `integrations/connectors/Repository.{List,ListOnline}` return a
  non-nil empty slice on empty result sets so JSON encoders emit `[]`
  instead of `null` — clients that pin to array shape (typed
  `ApplicationFromApi[]` / `ConnectorInstanceFromApi[]`) no longer
  crash on first refresh of an empty registry.

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
