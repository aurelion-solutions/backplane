# worker

Stand-alone orchestrator runner. Claims pending pipeline runs from
Postgres via `SELECT ... FOR UPDATE SKIP LOCKED`, executes their
steps through the in-process action registry, and writes status
back through `orchestrator.Service`.

## What this binary IS

- The execution path for every pipeline step. `cmd/backplane`
  schedules and matches; `cmd/worker` runs.
- The home of every action handler that touches PG. Engines
  register their handlers here at composition time; the registry
  validates args (JSON Schema) and dispatches with the
  `ActionContext` the step needs.
- Horizontally scalable. Run N processes; each opens E executor
  slots. Slots compete for the same pending queue — at-most-once
  delivery is enforced by `SKIP LOCKED` plus the status-guarded
  UPDATE inside `Service.ClaimPendingRun`.
- A producer of domain envelopes. The runner threads an
  `events.Sink` (`aurelion.events` exchange) into every
  `ActionContext`. Handlers that emit envelopes — engine actions and
  `noop.emit` — publish through it.

## What this binary IS NOT

- An HTTP server. Operator access goes through `cmd/backplane`.
- A scheduler. The schedule loop (`core/orchestrator/beat`) lives
  in `cmd/backplane`.
- A queue consumer for ingest or AuthZ traffic. Those are
  `cmd/ingester` and `cmd/pdp` respectively.

## Slot lifecycle

Each slot registers itself in `worker_slots` with a heartbeat-ed row
keyed by `(host, pid, slot_index)`. The reclaim sweep in backplane
re-queues steps whose slot stopped heartbeating beyond the configured
threshold — a hard crash takes seconds to recover, not until the
next schedule firing.

## Required env

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `AURELION_WORKER_SLOTS` | no | `4` | Executor slots per process. Bump on big boxes; cap by PG pool size. |
| `AURELION_WORKER_TAGS` | no | empty | Comma-separated tag list. Steps with a `worker_tag` requirement only claim against a worker carrying that tag. Empty means "every untagged step". |
| `AURELION_SECRET_PROVIDER` | no | `file` | Secret backend selector. |
| `AURELION_SECRETS_FILE` | no | `.secrets.json` | File-backend path. |

PG DSN, MQ URL, cartridge root come from the secret manager.

## Composition

Engines self-register their actions in this process's `main.go`. The
worker is the single composition root for every action — there is no
"AuthZ-only worker" or "ingest-only worker" today; one binary
carries them all and lets the orchestrator route by step tag.
