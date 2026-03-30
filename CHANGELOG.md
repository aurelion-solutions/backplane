# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
this project adheres to [Semantic Versioning](https://semver.org/).

## [0.3.0] — 2026-03-30

### Added

#### Inventory layer (`internal/inventory/`)

- New `inventory/` layer between `platform/` and `engines/` for the
  core domain entities Aurelion governs. The shape diverges from kernel
  in two important ways:
  - **Employment instead of Employee.** A single human can hold several
    concurrent or sequential masks at the same legal entity (e.g.
    full-time developer + part-time QA), each with its own org unit,
    manager, and access posture. Each mask is a row in
    `employments`; the canonical human is a `persons` row. The kernel's
    closed-vocabulary `EmployeeStatus` enum is replaced by
    `Employment.code` — a tenant-defined free-text label such as
    `active`, `probation`, `maternity_leave`, `notice_period`,
    `sabbatical`. Every company labels their working states
    differently, and the platform no longer pretends otherwise.
  - **Principal as the unified identity row.** What kernel calls
    `Subject` is here `Principal` — the canonical IAM term. Principal
    is the single point where access decisions land: it points at one
    body (employment / workload / customer) via partial FK columns
    and carries two orthogonal axes — `status` (lifecycle posture) and
    `is_locked` (operational/admin access lock). is_locked lives ONLY
    here; no employment / workload / customer table carries its own
    lock column. Locking any kind of identity is one and the same
    operation: `POST /principals/:id/lock`.
- `inventory/shared/` — vocabulary constants: `PrincipalKind`
  (`employment`, `workload`, `customer`), kind-specific status enums
  (`WorkloadStatus`, `CustomerStatus`; employment status is free
  text), `CustomerTenantRole`, `CustomerPlanTier`, plus the
  routing-key catalog for every event the inventory layer emits.

#### Slice — persons

- `Person` domain entity with `(id, external_id, full_name)`, unique
  on `external_id`, plus `PersonAttribute` 1→N (cascade) with unique
  `(person_id, key)` — stable per-human attributes (name, primary
  email, phone).
- REST surface under `/api/v0/persons`: list (paginated), create,
  get by id, bulk upsert (cap 500), list/add/remove attributes.
- Events: `inventory.person.{created, bulk_upserted,
  attribute_added, attribute_removed}`.

#### Slice — org_units

- `OrgUnit` self-referencing tree with `external_id`, `name`,
  `parent_id` (SET NULL), `description`, `is_internal`, `created_at`,
  `updated_at`. Internal hierarchy (seeded by deployment) and external
  hierarchies (synced via REST) coexist.
- Service enforces the `is_internal` invariant in code (no SQL
  trigger): API-created nodes are external, internal rows reject
  patch/delete, an external child may not attach to an internal parent.
- REST under `/api/v0/org-units`: list, create, bulk upsert with
  two-pass parent resolution by `parent_external_id`, get, patch,
  delete.
- Events: `inventory.org_unit.{created, updated, deleted,
  bulk_upserted}`.

#### Slice — employments

- `Employment` is a period of work for a single Person: `id`,
  `person_id` (FK CASCADE), `code` (free text, varchar 64),
  `start_date`, `end_date` (nullable = open), `org_unit_id` (SET NULL),
  `description`, timestamps. Partial unique index
  `WHERE end_date IS NULL` for active-employment lookups.
- `EmploymentAttribute` 1→N — period-specific tags (job_title,
  manager_external_id, headcount allocation). Stable per-human
  attributes live on PersonAttribute instead.
- REST surface under `/api/v0/employments`: list, create (with
  arbitrary code), bulk upsert (resolves person/org_unit by
  external_id), get, aggregate patch with `changes`-event, `POST
  /employments/:id/end` (sets end_date and emits ended), list/add/
  remove attributes; plus person-scoped listings under
  `/api/v0/persons/:personID/employments(/active)`.
- Code transitions trigger Principal status recompute via a
  cross-slice adapter; ended employments do the same so a terminated
  mask becomes inactive on its Principal.
- Events: `inventory.employment.{created, updated, ended,
  bulk_upserted, attribute_added, attribute_removed}`.

#### Slice — workloads

- `Workload` is a non-human identity body (service accounts, machine
  identities): id, external_id, name, description,
  `owner_employment_id` (SET NULL — owned by a specific Employment
  mask, not the human, so a workload tied to the developer mask goes
  with the developer mask), `application_id` (SET NULL),
  `WorkloadAttribute`. No is_locked column — that's on Principal.
- REST under `/api/v0/workloads`: list, create, bulk upsert, get,
  patch (no expire endpoint — locking goes through Principal), list/
  add/remove attributes.
- Events: `inventory.workload.{created, updated, bulk_upserted,
  attribute_added, attribute_removed}`.

#### Slice — customers

- `Customer` end-user body: id, external_id, email_verified,
  tenant_id, tenant_role (`admin|member|viewer`), plan_tier
  (`free|basic|pro|enterprise`), mfa_enabled, description,
  timestamps. `CustomerAttribute` 1→N with its own timestamps. No
  is_locked column — that's on Principal.
- Strict 3-field PATCH (email_verified, mfa_enabled, plan_tier).
  PATCH emits a single `updated` event listing the sorted set of
  changed field names. email_verified transitions trigger Principal
  status recompute.
- REST under `/api/v0/customers`: list, create, bulk upsert, get,
  patch, list/add/remove attributes.
- Events: `inventory.customer.{created, updated, bulk_upserted,
  attribute_added, attribute_removed}`.

#### Slice — employee_records (records, attribute mappings, matches, resolver)

- `EmployeeRecord` source-side row keyed on `(application_id,
  external_id)`, with cascading `EmployeeRecordAttribute`.
- `EmployeeProviderAttributeMapping` — per-application mapping from
  a record source key to a **canonical Person attribute key**
  (`person_key`), with `is_determinator` (drives resolver lookup) and
  `allow_upstream` (peer-record traversal edge). Unique on
  `(application_id, employee_record_key)`.
- `EmployeeRecordMatch` — 1:1 with EmployeeRecord, binding a record
  to a canonical **(Person, Employment)** pair plus a flag for
  whether the bridge was a direct determinator or upstream peer.
- REST under `/api/v0/employee-records`: list, create, bulk upsert
  (application referenced by `application_code`), get, list/add/
  remove attributes, manual match management
  (`GET/PUT/DELETE /employee-records/:id/match`), automated resolve
  (`POST /employee-records/:id/resolve`). Mapping CRUD lives under
  `/api/v0/applications/:appID/employee-record-mappings` and
  `/api/v0/employee-record-mappings/:id`.
- Resolver (`resolver.go`) ports the kernel matching algorithm to
  the new identity model:
  - Pass 1 — direct determinator (ANY-match). For each
    `is_determinator=true` mapping whose source key is present, look
    up a Person by `(person_key, value)`; if none exists, materialise
    a fresh Person seeded with the determinator attribute AND a fresh
    Employment (`code='active'`, `start_date=today`, no end_date) as
    the mask the record binds to.
  - For an existing Person, the resolver picks the first currently-
    active Employment as the binding mask
    (`PrimaryEmploymentForPerson`).
  - Pass 2 — upstream peer traversal: walk peer records sharing a
    `(key, value)` under an `allow_upstream=true` mapping, recurse
    with a visited-set to guard cycles.
  - Non-determinator mapped attributes propagate to the canonical
    Person on every successful match.
- The resolver itself never writes EmployeeRecordMatch — the service
  does, then emits
  `inventory.employee_record.{matched, unmatched}`.
- Events: `inventory.employee_record.{created, bulk_upserted,
  attribute_added, attribute_removed, matched, unmatched}`.

#### Slice — principals

- `Principal` polymorphic identity row over Employment / Workload /
  Customer with a `kind` discriminator, three partial FK columns
  (`principal_employment_id`, `principal_workload_id`,
  `principal_customer_id`), kind-specific `status` vocabulary,
  `is_locked` boolean, and `(kind, external_id)` uniqueness.
- Check constraints in the migration: exactly one `principal_*_id`
  set, `kind` ↔ matching FK, `status` ∈ kind vocabulary (employment
  status accepts any non-empty 64-char string; workload + customer
  bound to their universal vocabularies). Partial unique indexes on
  each `principal_*_id` enforce 1:1 body binding.
- `status_derivation.go` derives lifecycle status from current body
  state:
  - employment → `Employment.code` verbatim (or `terminated` when
    the row is gone)
  - workload   → `active` when the row exists, `expired` otherwise
  - customer   → `active` when email_verified, `registered` otherwise
  Lock is intentionally NOT part of derivation; it is a separate axis
  set explicitly via `POST /principals/:id/lock`.
- `RecomputeForBody(kind, bodyID)` is the cross-slice entry point:
  employments / customers call it on lifecycle / verification changes;
  it diffs current vs derived status and writes-back + emits
  `inventory.principal.status_recomputed` only on actual change.
- REST under `/api/v0/principals`: list, create (with derive-on-omit),
  get, `POST /:id/lock` (with optional reason), `POST /:id/unlock`.
  Lock/unlock are idempotent: re-locking an already-locked principal
  emits no event.
- Events: `inventory.principal.{created, status_recomputed, locked,
  unlocked}`.

#### Migrations

- Seven new bun migrations applied as a single group: `persons`,
  `org_units`, `employments` (`person_id`, `code`, `start_date`,
  `end_date`, partial index on active rows), `workloads`
  (`owner_employment_id`), `customers`, `employee_records` (+ record
  attribute / provider attribute mapping with `person_key` / 1:1
  match table with `person_id + employment_id`), `principals` (+
  `principal_attributes`, full check-constraint set, `is_locked`
  column, partial unique indexes on each body column).
- Eighth migration backfills `created_at` / `updated_at` columns on
  `persons` and `workloads` (DEFAULT NOW()) plus
  `(updated_at DESC)` indexes so paginated lists are stably ordered
  newest-first.

#### Additional endpoints

- `GET /api/v0/employee-record-matches` — returns every
  `EmployeeRecordMatch` row in one shot so clients can enrich a
  records list with its resolved (person, employment) without N+1
  per-record lookups.
- `GET /api/v0/principals/:id/attributes` — exposes the existing
  `principal_attributes` table (the row-level cross-body tagging) over
  REST, mirroring the persons / employments / workloads attribute
  surfaces.

### Changed

- `cmd/backplane/main.go` — composition root wires eight inventory
  slices end-to-end with cross-slice adapters that keep each slice
  decoupled:
  - `persons.Service`, `org_units.Service`, `employments.Service`,
    `workloads.Service`, `customers.Service`,
    `employee_records.Service` + `Resolver`, `principals.Service`.
  - Adapters bridge persons ↔ employments, org_units ↔ employments,
    applications ↔ workloads/employee_records, employments ↔
    workloads (owner-checks) and employee_records (employment
    membership), employments/customers ↔ principals (status
    recompute), and the employee_records resolver to a composed
    persons + employments API (the resolver materialises Person +
    Employment in one shot when no canonical row exists yet).
  - All inventory routes mounted under `/api/v0`.
- `internal/inventory/` replaces the previous empty
  `internal/domain/.gitkeep` placeholder.
- Persons / workloads / customers / principals List repositories now
  sort by `(updated_at DESC, id ASC)` instead of `external_id ASC`,
  so paginated UIs see the freshest rows first with stable
  tiebreaking. Other slices keep their previous orderings.
- Persons and Workloads gain `created_at` / `updated_at` columns and
  their services stamp `now()` on Create / Patch / BulkUpsert paths
  (workloads also propagate `updated_at` through the conflict upsert
  set).

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
