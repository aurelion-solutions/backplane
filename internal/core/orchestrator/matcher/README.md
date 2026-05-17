# matcher

MQ event consumer that drives two effects against the orchestrator:

1. **Waiter resolution.** A `wait_for_event` step parked in
   `pipeline_event_waiters` resolves when an inbound event matches
   its `routing_key` and the `match` JSONB predicate is contained in
   the event payload. Resolution goes through the same
   `Service.ResolveEventWaiter` the HITL endpoint uses, so manual
   and event-driven resolution share one code path.
2. **MQ-trigger firing.** Pipelines with `triggers: [{type: mq, ...}]`
   whose routing key matches the delivery and whose `match` predicate
   is contained in the payload start a new pipeline run with the
   delivery payload as `args`.

## Layout

| File | Role |
|---|---|
| `matcher.go` | Consumer wiring + dispatch loop |
| `match.go` | Predicate-containment check used by both effects |
| `doc.go` | Package overview |

## Predicate semantics

A predicate matches a payload iff every key in the predicate is
present in the payload and the values are deep-equal. Missing payload
keys disqualify the match; extra payload keys are ignored.

This is intentionally narrower than full JSONPath / JMESPath — it
keeps the match table small, the dispatch O(N) on the loaded
catalog, and the rules predictable.

## What this package does NOT do

- Execute steps. The matcher only triggers — execution is `runner`.
- Persist events. Event durability is owned by the broker.
- Validate routing-key shape. Bad routes from cartridges fail at
  load time in `loader`, not here.
