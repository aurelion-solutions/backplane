# orchestrator/actions

Pipeline-primitive action handlers that belong to the orchestrator
itself rather than to any engine. They are domain-free, never touch a
business table, and can appear in any pipeline.

## Packages

[`noop`](noop/) — pipeline-shape primitives:

- `noop.echo` — copies the input message through; keeps the graph
  valid before a real producer exists.
- `noop.sleep` — bounded wait (≤ 60s) that honours cancel; used for
  pacing, heartbeat tests, cancel propagation.
- `noop.fail` — deliberate handler error with a supplied message;
  populates the Failed sidebar bucket and exercises reclaim / retry.
- `noop.constant` — returns an arbitrary JSON object verbatim;
  stubs a producer step so downstream `${steps.X.result.value}`
  expressions resolve.
- `noop.emit` — publishes a domain envelope through the runner's
  `events.Sink`; lets a pipeline raise the event a downstream
  `wait_for_event` step waits on, without an external harness.

## What lives here

- Actions whose behaviour is a pipeline-shape concern: returning the
  input, parking the worker for a bounded duration, raising a fixed
  error, etc.
- Anything a cartridge author may legitimately reach for to stub,
  pace, or branch a pipeline without pulling in engine logic.

## What does NOT live here

- Engine actions — those live under
  `internal/engines/<engine>/actions/<action>/` and own a domain
  contract.
- Test fixtures. Smoke pipelines that use these actions are real
  artefacts shipped in `cartridges/popular/pipelines/`; the actions
  are production-quality, not test-only.
