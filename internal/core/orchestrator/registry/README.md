# registry

In-memory keyed registry of engine actions. Engines register handler
functions at composition time; the runner dispatches by
`(engine, action)` pair.

## What Register gives you

```go
registry.MustRegister(reg,
    "policy_assessment", "assess",
    assess.Schema(),          // JSON Schema for Args
    assess.New(deps),         // registry.Handler[Args, Result]
)
```

For each registered pair the registry owns:

- **JSON Schema validation** of the raw args map (via
  `santhosh-tekuri/jsonschema`).
- **Args unmarshalling** from `map[string]any` to the handler's
  typed input struct.
- **Handler invocation** with the supplied `ActionContext` (carries
  `ctx`, `tx`, `log`, `events` sink, `pipeline_run_id`, `step_run_id`,
  `attempt`, `worker_id`).
- **Result marshalling** back to `map[string]any` for persistence
  on the step row.

The runner is the only caller of `Dispatch` in production; tests
call it directly to exercise an action in isolation.

## Layout

| File | Role |
|---|---|
| `registry.go` | `Registry`, `Register`, `Dispatch` |
| `context.go` | `ActionContext` shape |
| `errors.go` | Typed errors surfaced to the runner |
| `doc.go` | Package overview |

## What this package does NOT do

- Wire actions. That's done at composition root (`cmd/backplane`,
  `cmd/worker`).
- Persist anything. Step row writes are the runner's job; the
  registry only returns the result map.
- Retry. A failed action propagates the error to the runner, which
  decides per the step's `retry` policy.
