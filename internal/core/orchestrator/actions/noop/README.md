# noop

Pipeline-shape primitives that are domain-free and safe to use anywhere.

| Action | Args | Result | Idempotent | Behaviour |
|---|---|---|---|---|
| `noop.echo`     | `{message: string}` | `{message: string}` | yes | Returns the input verbatim. |
| `noop.sleep`    | `{sleep_millis: int}` | `{slept_millis: int}` | yes | Parks the worker for the requested duration. Bounded `0 < ms <= 60_000`. Honours cancel via `ctx.Ctx`. |
| `noop.fail`     | `{message: string}` | — | yes | Raises a deliberate handler error wrapping `ErrDeliberate`. Empty message is rejected. |
| `noop.constant` | `{value?: object}` | `{value: object}` | yes | Copies an arbitrary JSON object verbatim. Omitting `value` yields `{}` — useful as a pure no-op step. |
| `noop.emit`     | `{event_type: string, correlation_id?: string, payload?: object}` | `{event_id, event_type, correlation_id}` | **no** | Publishes a domain envelope through `ActionContext.Events`. Falls back to the pipeline run ID for `correlation_id`. Requires `ctx.Events` — handler returns an error if it is nil. |

`noop.emit` is the only side-effecting action: a retried dispatch
produces a fresh envelope with a new `event_id`, so the registry
marks it non-idempotent and the runner will not retry it on
transient failures.

## When to use

- **Pipeline shape**: stand up a working pipeline before its real
  actions exist — `echo` and `constant` keep the graph valid and let
  downstream template expressions resolve.
- **Pacing**: `sleep` is the canonical way to add a deliberate wait.
- **Deliberate failure**: `fail` populates the Failed sidebar bucket
  in demos and exercises reclaim / retry paths in tests.
- **Self-contained event loops**: `emit` lets a pipeline publish the
  envelope that a downstream `wait_for_event` step waits on —
  no external curl / MQ harness needed for smoke runs.
- **Smoke cartridges**: `cartridges/popular/pipelines/smoke.*.yaml`
  use these to validate the runner end-to-end without involving any
  engine.

## Why they belong to the orchestrator, not an engine

They have no domain. They never read or write a business table, never
participate in a business lifecycle. Their contracts are pipeline-shape
concerns (input → output, duration, cancellation, deliberate failure,
envelope emission). Engines, by contrast, own domain entities and
lifecycles.

## Registration

`Register(r *registry.Registry)` is called by both composition roots
(`cmd/backplane`, `cmd/worker`) so the loader can resolve their
qualified names and the runner can dispatch them. The runner threads
`events.Sink` into `ActionContext.Events`; `noop.emit` depends on it.

## Bounds

- `noop.sleep` caps at **60_000 ms**. A misconfigured pipeline cannot
  park a worker slot longer than that without raising.
- `noop.fail` rejects an empty message — a misconfigured cartridge
  surfaces loudly instead of producing a blank failure.
- `noop.emit` rejects an invalid `event_type` (empty or malformed)
  via `events.NewEnvelope` validation.
- `noop.constant` has no bounds — it is a pure copy.
