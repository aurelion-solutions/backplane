# access_generate.run

Thin orchestrator wrapper around `engine.Recompute`. One pipeline
step inside `access_generate.run` re-derives the desired set of
initiatives for a single principal and persists the diff.

Pair: `("access_generate", "run")`.

## Inputs (`Args`)

| Field | Required | Meaning |
|---|---|---|
| `principal_id` | required | Principal whose initiatives are being rebuilt. |
| `application_id` | optional | Limit the recompute to one application. |
| `capability_id` | optional | Limit the recompute to one capability inside the chosen application. |

Both filters empty → the full `(principal, ∀ applications, ∀ capabilities)`
scope is rebuilt.

## Output (`Result`)

```go
type Result struct {
    CreatedCount    int
    TombstonedCount int
    EventsEmitted   int
}
```

`CreatedCount + TombstonedCount` is the size of the diff between the
desired set the engine computed and the active set already in PG.
`EventsEmitted` covers the post-commit `inventory.initiative.created`
/ `inventory.initiative.tombstoned` MQ publications.

## What it does

The action validates `principal_id`, packages the optional filters
into `engine.RecomputeFilter`, and calls `engine.Recompute`. All the
real work — collecting planned initiatives from inheritance /
requested / delegated sources, diffing against current rows,
persisting the change set in one transaction, publishing the events
after commit — lives in the engine. See the engine README at
[`../../README.md`](../../README.md) for the pipeline stages.

## Composition

`cmd/worker` wires the action through `run.Register(reg,
engine.Engine)`. The engine is shared across actions; this wrapper
holds no state of its own.
