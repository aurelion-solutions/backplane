# inventory_normalize

Not a standalone engine. This is a **package of orchestrator actions
for the worker** — no dispatcher of its own, no runtime. The worker
already executes pipeline steps from its action registry by
`(engine, action)` — we just register normalize actions there.

## Lifecycle

```
inventory.ingest.batch_received (event on aurelion.events)
        │
        ▼
matcher (external component in backplane's composition root)
        │
        ├─ walks the pipeline catalog for MQ triggers with
        │     routing_key = inventory.ingest.batch_received and a
        │     match predicate that satisfies the payload
        │     (e.g. match: {dataset_type: account})
        ├─ extracts args_from_payload (batch_id, source, lake_ref)
        │
        ▼
orchestrator.Service.CreateRun (one-step PipelineRun)
        │
        ▼
worker slot claims it, executes the action
        │
        ├─ action reads the right records from the lake
        │     (storage.ReadBatch(lake_ref))
        ├─ applies its logic (resolve / project / upsert)
        ├─ writes its Postgres tables via ctx.Tx
        └─ returns Result; runner commits Tx
```

Triggers:

- `inventory.ingest.batch_received` event (the main path — via the
  matcher)
- manual replay by creating a PipelineRun with the same step (after
  changing rules)

## Action registration

In backplane's composition root:

```go
worker.RegisterAction("inventory.normalize.account",             actions.NewAccount(deps))
worker.RegisterAction("inventory.normalize.employee",            actions.NewEmployee(deps))
worker.RegisterAction("inventory.normalize.access_grant_record", actions.NewAccessGrantRecord(deps))
```

An action is a plain Go handler implementing the worker's Action
interface. No `inventory_normalize` infrastructure of its own.

## Current actions

| Action | Dataset_type | What it does | Writes to |
|---|---|---|---|
| [`account`](actions/account/) | `account` | flat upsert per `(application_id, username)` | `accounts` |
| [`employee`](actions/employee/) | `employee` | determinator resolution + cross-app upstream attach | `persons`, `person_attributes`, `employments` |
| [`access_grant_record`](actions/access_grant_record/) | `access_grant_record` | projection through `CapabilityMapping` | `capability_grants` |

`access_artifact` deliberately has **no dedicated action**: billions
of resource descriptions are not worth duplicating into Postgres.
Resource attributes are read directly from the lake via DuckDB scan
at projection time.

## Events

| Type | When |
|---|---|
| `inventory.normalize.<action>.completed` | action finished successfully |
| `inventory.normalize.<action>.failed` | action crashed — batch stays in the lake, can be replayed |

Actions may emit their own per-record events — see each action's
README.

## What it does NOT do

- write to the lake (read-only)
- attach account → principal — that is a separate engine run AFTER
  normalize
- start other pipelines / actions
- validate the payload — that is each action's concern
- aggregate grants / counts — that is analytics
