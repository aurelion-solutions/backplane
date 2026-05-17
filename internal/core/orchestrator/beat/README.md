# beat

Periodic scheduler and timeout sweeper for the orchestrator. Runs
inside `cmd/backplane` as a single-leader loop guarded by
`pg_try_advisory_lock`.

## Two responsibilities

1. **Schedule fire.** Pipelines with `schedule` triggers fire when
   their cron / `every` window has elapsed since the last fire. The
   "firepoint" abstraction owns the per-window dedupe so a flapping
   leader does not double-fire.
2. **Waiter sweep.** `pipeline_event_waiters` rows whose
   `expires_at` is in the past flip to `failed_timeout` and the
   parked step + parent run transition accordingly.

## Layout

| File | Role |
|---|---|
| `beat.go` | `Beat` struct + `Run(ctx)` loop |
| `firepoint.go` | Per-trigger fire-window arithmetic |
| `doc.go` | Package-level overview |

## Cadence

A single tick — fire the schedules due, sweep the expired waiters —
runs every `tickInterval` (default `5s`, configured at construction).
Each tick is one transaction per concern, scoped via advisory lock
so a multi-replica deployment only has one beat at a time.

## What this package does NOT do

- Execute pipeline steps. That's `core/orchestrator/runner`.
- Listen to MQ. That's `core/orchestrator/matcher`.
- Define pipelines. Definitions come from `core/orchestrator/loader`
  via the in-memory `Catalog`.
