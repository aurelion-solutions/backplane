# runner

The orchestrator's work loop. Runs inside `cmd/worker` — one slot
per goroutine, N goroutines per process — and claims pending runs
from Postgres via `SELECT ... FOR UPDATE SKIP LOCKED`.

## Three-transaction-per-step protocol

Step execution splits into three transactions so a worker crash at
any point leaves the system in a recoverable state:

- **Tx A — claim.** Take the run with `FOR UPDATE SKIP LOCKED`,
  flip its status to `running`, stamp `worker_id` and
  `last_heartbeat_at`. Commit.
- **Tx B — persist step.** Insert the `step_runs` row with
  `status = running` so a later recorder transaction knows the step
  exists. Commit.
- **Tx C — record outcome.** Inside the action's own transaction (the
  one the action uses to write its domain rows), update the step
  row with status / result / error. Commit.

Heartbeats run on a separate goroutine and bump `last_heartbeat_at`
on the runs and step_runs rows; the reclaim sweeper repossesses any
row whose worker stopped heartbeating.

## Templates

Step args support `${args.X}` and `${steps.<step>.result.<path>}`.
Resolution happens just before dispatch via `templates.go`. Missing
references fail the step deterministically — the resolver never
inserts empty strings or nulls behind a missing key.

## Layout

| File | Role |
|---|---|
| `runner.go` | Claim loop + step execution |
| `worker.go` | Goroutine slot lifecycle, heartbeat |
| `templates.go` | `${args.X}` / `${steps.S.result.X}` resolution |
| `duration.go` | Cron / `every` duration helpers shared with `beat` |
| `doc.go` | Package overview |

## What this package does NOT do

- Schedule pipelines. That's `beat`.
- Match MQ events into runs. That's `matcher`.
- Define what an action does. The registry holds the handler; this
  package only invokes it.
- Persist domain rows directly. Actions own their writes through
  their own transactions; the runner only writes the step row.
