# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
this project adheres to [Semantic Versioning](https://semver.org/).

## [0.4.0] — 2026-04-08

### Added

#### Worker slot registry + `/workers` endpoint (`internal/core/orchestrator/runner/`, `internal/core/orchestrator/routes_workers.go`)

- New `worker_slots` table — one row per live runner slot,
  upserted on slot start, refreshed by a dedicated 5 s
  heartbeat goroutine, deleted on graceful shutdown. The
  `/workers` endpoint reads this registry directly (not
  derived from `pipeline_runs`), so idle slots are visible
  alongside busy ones — derived-from-runs view never showed
  idle workers and that was the gap.
- `WorkerStaleThreshold = 15 s` (3× heartbeat interval)
  filters rows whose heartbeat is older — a crashed process
  disappears within one threshold window.
- Readonly process tags via `AURELION_WORKER_TAGS=gpu,llm,…`
  env var. Worker startup parses the CSV (trim + dedupe),
  shares the same set across every slot of the process, and
  upserts into `worker_slots.tags TEXT[]`. Purely
  informational for now — surfaced in the Studio overview
  panel as chips next to the PID header.
- `GET /api/v0/workers` returns the registry joined with
  per-`worker_id` active-run aggregates (count, earliest
  start, representative current run + pipeline) so the
  Studio overview can render busy/idle status + drill into
  the currently-held run without N+1 lookups.

#### Orchestrator MQ matcher (`internal/core/orchestrator/matcher/`)

- Async RabbitMQ consumer that drives two effects per delivery on
  `aurelion.events`:
  1. Waiter resolution: every wait_for_event step parked in
     `pipeline_event_waiters` whose `match` JSONB is contained in the
     incoming payload (`match <@ payload`) is resolved via the same
     `Service.ResolveEventWaiter` the HITL endpoint uses, so the
     parked run wakes up and the runner re-claims it.
  2. MQ-trigger firing: every pipeline definition with a
     `type: mq` trigger whose `routing_key` matches the delivery AND
     whose `match` predicate is contained in the payload spawns a
     fresh run via `Service.CreateRun(TriggerMQ)`. The partial-UNIQUE
     idempotency index plus `content_hash` dedupe automatically
     squashes RabbitMQ at-least-once redeliveries — re-firing the
     same event for the same in-flight run returns the existing row.
- Effects (1) and (2) run in **independent transactions** so a
  failure in one cannot roll the other back.
- Cluster-wide single-active: matcher holds a session-level
  `pg_advisory_lock` (key `0x4155_5245_4C4D_4154` = "AURELMAT") on a
  dedicated PG connection. Siblings that cannot acquire become warm
  standbys retrying every second.
- New bootstrap settings:
  `RabbitMQ.MatcherQueue` (default `aurelion.orchestrator.matcher`) —
  the durable queue the matcher binds to `aurelion.events` with
  catch-all routing key.

#### Orchestrator Beat (`internal/core/orchestrator/beat/`)

- Periodic scheduler + waiter timeout sweeper, ticking every 10 s.
- Per-tick `pg_try_advisory_lock` (key `0x4155_5245_4C42_4541` =
  "AURELBEA7") so several backplane replicas can run the goroutine
  without double-firing schedules.
- `PreviousFirePoint(now, cron, every)` returns the most recent
  fire-point of a schedule. `every` uses an epoch-anchored floor for
  determinism across restarts; `cron` is parsed by
  `robfig/cron/v3`. `Service.IsScheduleAlreadyFired` dedupes within
  the cron window using a count over `pipeline_runs` filtered by
  `trigger_source=schedule` AND `created_at >= fire_point`.
- Each tick also walks `pipeline_event_waiters` for `expires_at < now`
  and transitions the parked step + run from `awaiting_event` →
  `failed_timeout` via the new `Service.ExpireEventWaiter`
  (`Repository.ScheduleAlreadyFired` is the matching new repo method).
- Launched as a goroutine from `cmd/backplane`; safe to run in every
  replica thanks to the advisory lock.

#### Orchestrator runner + run-mutating REST surface (`internal/core/orchestrator/runner/`, `internal/core/orchestrator/routes_runs.go`, `cmd/worker/`)

- `runner.Runner` is the PG-claim work loop driver. Each instance owns
  one slot inside `cmd/worker`; the loop reclaims stale runs at the
  head of every tick, claims one pending run via the Service's
  `SELECT … FOR UPDATE SKIP LOCKED` + guarded UPDATE, then drives every
  step. Three-transaction-per-step protocol: Tx A claims, Tx B inserts
  the StepRun (committed so failure transactions can see it), Tx C
  runs the action and commits the success transition atomically. On
  action error a fresh Tx D writes step + run failure.
- Heartbeat goroutine refreshes `pipeline_runs.last_heartbeat_at`
  every 3 s and watches for a status flip to `cancelling`; when it
  spots one it cancels the action's context so the handler unwinds
  cleanly, after which the run is marked `cancelled`.
- Resume on re-claim: the runner loads every prior `completed` step
  row before executing the loop and skips steps already done. This
  closes the loop with `wait_for_event` parking — once a waiter is
  resolved, the worker re-claims the run and continues from the next
  step instead of re-running already-done work (which would otherwise
  hit `uq_step_runs_run_step_attempt`).
- Mutating REST surface under `/api/v0`:
  - `POST /pipelines/{name}/runs` — create a run; 201 on fresh insert,
    200 on idempotency dedupe.
  - `GET /pipelines/{name}/runs` — list runs for one pipeline.
  - `GET /pipelines/runs` — global list with `?pipeline=`, `?status=`,
    `?limit=`, `?offset=` filters.
  - `GET /pipelines/runs/{id}` — detail + ordered steps.
  - `GET /pipelines/runs/{id}/steps` — every step attempt for one run.
  - `GET /pipelines/runs/{id}/steps/{step}` — latest attempt for a
    named step.
  - `POST /pipelines/runs/{id}/cancel` — synchronous for pending /
    awaiting_event runs, asynchronous (`cancelling`) for running ones.
  - `POST /pipelines/runs/{id}/retry` — terminal-only.
  - **`POST /pipelines/runs/{id}/steps/{step}/resolve`** — the HITL
    endpoint. An operator passes `{payload: {…}}`; the same
    `Service.ResolveEventWaiter` the matcher will use later marks the
    step complete, deletes the waiter, and re-activates the run.
- `cmd/worker` is now a real runner-bootstrap process: it boots the
  same composition root as `cmd/backplane` (secrets → postgres →
  cartridges → action registry → catalog), spawns
  `AURELION_WORKER_SLOTS` goroutines (default 4), and waits up to 60 s
  for them to drain on SIGINT/SIGTERM.

#### Pipeline discovery + read-only REST surface (`internal/core/orchestrator/`)

- `orchestrator.Catalog` is the immutable in-memory snapshot of every
  pipeline definition discovered at startup. `LoadFromCartridges`
  iterates the configured cartridge ids (or every cartridge the
  provider knows about when the list is empty) and feeds each
  `<cartridge>/pipelines/` directory through the loader. Duplicate
  pipeline names across cartridges fail boot fast.
- `BuildMergedSchema` deep-copies the embedded grammar and injects
  per-action arg / result schemas under
  `$defs.action_args["<engine>.<action>"]` and
  `$defs.action_results[…]`. Merge is purely additive — existing
  schema entries are preserved.
- `BuildActionCatalogue` enumerates every registered action with its
  idempotent flag and both schemas; consumed by `GET /api/v0/actions`.
- New read-only REST surface:
  - `GET /api/v0/pipelines` — sorted summary list.
  - `GET /api/v0/pipelines/{name}` — full definition.
  - `GET /api/v0/actions` — registered action catalogue.
  - `GET /.well-known/pipeline-schema.json` — merged JSON Schema for
    the VSCode YAML completion in aurelion-engineering-studio.
- Composition root wires the catalog after `cartridges.Provider` and
  before `webserver`. Action-ref validation is intentionally OFF
  (`loader.Loader.Actions = nil`) — flip it on once the engine
  packages register their actions.

#### Orchestrator state tables + Service (`internal/core/orchestrator/`)

- Migration `20260530090000_orchestrator` creates the three pipeline
  state tables — `pipeline_runs`, `step_runs`, `pipeline_event_waiters`
  — plus four PG enum types
  (`pipeline_run_status`, `step_run_status`, `pipeline_event_waiter_status`,
  `pipeline_trigger_source`). Partial UNIQUE index
  `uq_pipeline_runs_inflight_idempotency` blocks duplicate in-flight runs
  for the same (pipeline_name, pipeline_version, content_hash) while
  retries (retry_of_run_id NOT NULL) and terminal rows bypass.
- `orchestrator.Service` is the sole writer to all three tables.
  Every status-changing UPDATE WHERE-guards on the expected source
  status; a 0-rowcount triggers refresh-and-branch logic rather than a
  silent retry. The full lifecycle: `CreateRun` with
  partial-UNIQUE dedupe + retry-of bypass, `CreateRetry` (terminal-only
  guard), `ClaimPendingRun` via `SELECT … FOR UPDATE SKIP LOCKED` +
  guarded `pending → running` UPDATE, `RefreshHeartbeat` /
  `ReclaimStaleRun` / `ListStaleRunIDs` (10 s hard-coded threshold),
  the `pending/awaiting_event/running` cancel branches with the
  cancel-vs-complete race silently transitioning to `cancelled`,
  step lifecycle (`CreateStepRun`, `MarkStepSucceeded`,
  `MarkStepFailed`, `MarkStepAwaitingEvent`), and the shared
  `ResolveEventWaiter` consumed by both the matcher and the HITL
  `POST /pipelines/runs/{id}/steps/{step}/resolve` endpoint.
- `Repository` interface keeps Service decoupled from bun for unit
  tests. `BunRepository` ships the production implementation;
  `memRepo` in tests covers idempotency dedupe, retry guards, all
  three cancel branches, complete-vs-cancelling race, waiter resolve
  + run re-activation, and oldest-first claim ordering.

#### Action registry + `noop` actions (`internal/core/orchestrator/registry/`, `internal/actions/noop/`)

- Generic in-memory action registry keyed by `(engine, action)`. Engines
  call `registry.Register[A, R](r, engine, action, idempotent, h)` at
  composition time. Args / result JSON Schemas are derived
  automatically from the handler's input / output struct definitions
  via `invopop/jsonschema`, then compiled with
  `santhosh-tekuri/jsonschema` for runtime validation.
- `Registry.Dispatch` does the full pipeline: args-schema validation →
  JSON-roundtrip into the handler's struct → handler invocation →
  result-schema validation → struct-to-map for storage. The runner
  (Step 6) is the only caller.
- `Registry.Has` implements `loader.ActionLookup` so Step 5's discovery
  step can flip on action-ref validation in the YAML loader.
- `internal/actions/noop` ships two trivial smoke actions —
  `noop.echo` and `noop.sleep` (bounded to 60 s; respects context
  cancellation) — used by the integration test harness and by the
  default smoke pipeline.

#### Pipeline grammar + loader (`internal/core/orchestrator/{grammar,loader}/`)

- Embedded JSON Schema 2020-12 grammar for pipeline YAML
  (`grammar/schema.json`) — the single source of truth for both the
  loader and the well-known endpoint (Step 5). `runs` is reserved as a
  pipeline name to avoid collision with the
  `/api/v0/pipelines/runs/...` route family.
- `loader.Loader` reads YAML, validates against the embedded grammar,
  then runs fail-fast checks: requires order (no forward / self /
  unknown deps), templating (`${args.X}` against declared properties
  and `${steps.S.result.X}` against the transitive requires closure),
  triggers (at most one schedule, mq `args_from_payload` keys +
  values shape).
- `LoadFile` / `LoadDir` / `LoadMany` cover single-file, directory,
  and multi-source loading. Each loaded definition carries an
  immutable `content_hash` = sha256 of canonicalised JSON so the
  runner can detect args drift.
- Optional `ActionLookup` hook (no-op by default; wired to the
  registry on Step 3) lets the loader reject step `(engine, action)`
  refs that aren't registered. Smoke-test passes for every YAML
  currently shipped in `cartridges/journey/pipelines/`.

#### Cartridges provider (`internal/core/cartridges/`)

- Source-agnostic provider for the external cartridges bundle that
  lives outside the backplane git repo (default path: `../cartridges`,
  overridable via `AURELION_CARTRIDGES_ROOT` env or the `cartridges`
  bootstrap secret). A cartridge is a directory containing
  `pipelines/*.yaml` and `policies/<bucket>/<rule>.meta.json` +
  `<rule>.rego` files. The provider only enumerates ids and exposes
  files on disk — it knows nothing about pipeline grammar or rego
  semantics.
- `Provider` interface (`List`, `Materialize`, `Policies`, `Pipelines`)
  plus `Manifest` projection of one `.meta.json` sidecar. `Factory`
  registers named provider constructors mirroring `storage.Factory` /
  `siem.Factory`. `FilesystemProvider` is the only registered source
  today; git / OCI / zip drop in next to it without touching consumers.
- Read-only REST surface under `/api/v0/cartridges`:
  - `GET /cartridges` — list every cartridge id with pipeline / policy
    counts.
  - `GET /cartridges/{id}` — detail with materialized root path.
  - `GET /cartridges/{id}/policies` — list of `Manifest` records.
  - `GET /cartridges/{id}/pipelines` — list of pipeline YAML files
    (filename + absolute path).
- `config.Cartridges` bootstrap section with sane defaults; existing
  deployments without a `cartridges` secret continue to boot.

## [0.3.0] — 2026-03-30

### Added

#### Inventory layer (`internal/inventory/`)

- New `inventory/` layer between `platform/` and `engines/` for the
  core domain entities Aurelion governs. Two foundational shapes:
  - **Employment, not Employee.** A single human can hold several
    concurrent or sequential masks at the same legal entity (e.g.
    full-time developer + part-time QA), each with its own org unit,
    manager, and access posture. Each mask is a row in
    `employments`; the canonical human is a `persons` row. Employment
    state is `Employment.code` — a tenant-defined free-text label
    (`active`, `probation`, `maternity_leave`, `notice_period`,
    `sabbatical`, …) so every company can label their working states
    in their own vocabulary without the platform pretending otherwise.
  - **Principal as the unified identity row.** `Principal` is the
    canonical IAM term and the single point where access decisions
    land: it points at one body (employment / workload / customer) via
    partial FK columns and carries two orthogonal axes — `status`
    (lifecycle posture) and `is_locked` (operational/admin access
    lock). `is_locked` lives ONLY here; no employment / workload /
    customer table carries its own lock column. Locking any kind of
    identity is one and the same operation:
    `POST /principals/:id/lock`.
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
- Resolver (`resolver.go`) two-pass matching:
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
  with `WithID` / `ID` / `Ensure` helpers, so service code can stamp
  events / log entries / RPC calls with one trace id.
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
  with their full column shape, indexes, and unique constraints.
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
