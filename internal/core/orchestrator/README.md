# core/orchestrator

The pipeline-runtime substrate. Owns the three state tables
(`pipeline_runs`, `step_runs`, `pipeline_event_waiters`), the
in-memory pipeline `Catalog`, the action `Registry`, and every code
path that mutates run state. Everything runtime-related either lives
here or wires through `Service`.

## Invariants

- **`Service` is the sole writer** of the three state tables. Engines
  read their own data but MUST NOT write to orchestrator state.
- **Every status-changing UPDATE WHERE-guards on the expected source
  status.** A zero-rowcount triggers refresh-and-branch logic
  (cancel-vs-complete race handling) — never a silent retry.
- **The `runner` sub-package is the only orchestrator code that calls
  `Commit` / `Rollback` on `bun.Tx`.** Service / Repository never do.
  Route handlers pass an `*bun.DB` and Service composes operations
  inside `RunInTx`.
- **Multi-replica safe by design.** `beat` uses `pg_try_advisory_lock`
  per tick; `matcher` holds a session-level `pg_advisory_lock`;
  `runner` slots use `FOR UPDATE SKIP LOCKED` on the claim. N replicas
  of backplane and N processes of worker can run side by side without
  coordination.

## What lives at the package root

| File | Role |
|---|---|
| `model.go` | `PipelineRun`, `StepRun`, `PipelineEventWaiter`, `WorkerSlot` rows + lifecycle enums (`RunStatus`, `StepStatus`, `WaiterStatus`, `TriggerSource`). |
| `repository.go` | `Repository` interface + bun-backed implementation. Pure persistence — no business logic. |
| `service.go` / `service_test.go` | `Service` — every state transition, run dedupe, reclaim sweep, event-waiter resolution. |
| `discovery.go` / `catalog_watcher.go` | `Catalog` (mutable, RWMutex-guarded, `Reload`-able) + `LoadFromCartridges` + `RunCatalogWatcher` (5 s mtime polling). |
| `errors.go` | Domain errors (`ErrNotFound`, `ErrIllegalTransition`, `ErrAlreadyTerminal`, …). |
| `hash.go` | Args canonicalisation + content hash used for run dedupe. |
| `schemamerge.go` | Merges per-step args schemas into one pipeline-wide schema for declarative validation. |
| `routes_definitions.go` | `GET /pipelines`, `GET /pipelines/:name`, source listings (Studio surface). |
| `routes_runs.go` | `POST /pipelines/:name/runs`, `GET /pipelines/runs/:id`, `POST .../{cancel,retry}`, HITL `resolve`. |
| `routes_workers.go` | `GET /workers` — registry + heartbeats. |

## Sub-packages

| Path | Role | Multi-replica story |
|---|---|---|
| `grammar/` | Embedded JSON Schema for pipeline YAML. | Static. |
| `loader/` | YAML parser + structural / templating / trigger validator. Builds immutable `*Definition` with content hash. | Stateless. |
| `registry/` | In-memory action registry. JSON-Schema-validated args + result. Engines `Register` at composition time. | Per-process. |
| `runner/` | Worker work-loop. Slot lifecycle, three-Tx-per-step protocol, heartbeat goroutine, reclaim sweep. | N processes × N slots, `FOR UPDATE SKIP LOCKED`. |
| `beat/` | Periodic scheduler + waiter-timeout sweep. | One-tick-at-a-time via `pg_try_advisory_lock` (`AURELBEA7`). |
| `matcher/` | MQ event consumer; resolves waiters + fires `type=mq` triggers. | Session-level `pg_advisory_lock` (`AURELMAT`); siblings become warm standbys. |

## Catalog hot reload

`Catalog` is mutable but goroutine-safe. `Reload(provider, loader,
ids)` swaps `defs` + `sources` under a write lock; `Get`, `All`, and
`Sources` take a read lock. Failure leaves the previous catalogue in
effect.

`RunCatalogWatcher(ctx, catalog, provider, loader, ids, root,
interval, log)` polls the cartridges root for `.yaml` mtime changes
(5 s default) and calls `Reload` on every diff. Wired up in both
`cmd/backplane` (so `beat` schedules and the `matcher` trigger map
stay fresh) and `cmd/worker` (so claimed runs see new step
definitions without a restart).

## Run lifecycle (happy path)

```
POST /pipelines/{name}/runs
  └─ Service.CreateRun
       INSERT pipeline_runs (status=pending, content_hash, args_hash)
       partial-UNIQUE dedupe → returns existing in-flight row if any
                              ─┐
                                ▼
runner.WorkLoop                Tx A: SELECT ... FOR UPDATE SKIP LOCKED
                                     UPDATE status pending→running
                                ▼
                                Tx B: INSERT step_runs (status=running)
                                ▼
                                Tx C: action.Handle(ctx, input)
                                       on success → UPDATE step+run terminals
                                       on error   → Tx C rollback,
                                                    Tx D writes failure
                                ▼
                                Service.EmitTerminalEvents
                                  (pipeline.run.completed / failed)
```

`wait_for_event` steps park the run in `pipeline_event_waiters` and
flip `status` to `awaiting_event`; `matcher` resolves them and the
runner picks the run back up on the next loop iteration.

## What this package does NOT do

- **No transport.** REST handlers live here only because they are
  thin (parse → call `Service` → return). HTTP framing belongs to
  `core/webserver`; MQ framing to `matcher` / `rabbitmq`.
- **No action implementations.** Actions live next to their owning
  engines (`internal/engines/...`, `internal/actions/noop`) and
  `Register` themselves at composition time.
- **No engine state.** Engines own their own tables and services;
  orchestrator only knows that an action runs and either succeeds
  or fails.
- **No queue durability for the matcher.** Each event is one shot —
  consumed once on the active replica. Durability of upstream events
  is `rabbitmq`'s concern.
