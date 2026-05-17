# events

Domain-event delivery layer. One `Sink` interface, one canonical
`Envelope`, several transport implementations behind it.

This is **not** process logging (`core/logger`) and **not** audit
logs (`platform/siem`). An Envelope is a statement of fact the rest
of the system reacts to — "an account changed", "a pipeline run
completed", "an initiative was tombstoned".

## Pieces

| File | Role |
|---|---|
| `envelope.go` | `Envelope` shape + `NewEnvelope(EnvelopeInput)` builder |
| `interface.go` | `Sink` interface — the single contract every consumer programs against |
| `mq.go` | `MQSink` — publishes Envelopes to RabbitMQ via `core/rabbitmq` |
| `tee.go` | `TeeSink` — fans an Envelope out to N child sinks |

## Envelope

```
{
  event_id:       uuid
  event_type:     string  // e.g. "inventory.employee.updated"
  occurred_at:    rfc3339
  correlation_id: string
  source:         string
  payload:        map[string]any
}
```

`event_id` is the natural dedup key for at-least-once delivery.
Consumers that need idempotency record `event_id` in their own
state.

## Routing

The `event_type` is used as the AMQP routing key on the
`aurelion.events` topic exchange. Producers do not declare
exchanges; that is done once at boot by `core/rabbitmq` from
config.

## Caller pattern

```go
err := sink.Publish(ctx, events.NewEnvelope(events.EnvelopeInput{
    EventType: "inventory.initiative.tombstoned",
    Source:    "engines/access_generate",
    Payload:   map[string]any{"id": id, "principal_ref": pid},
}))
```

The builder stamps `event_id`, `occurred_at`, and
`correlation_id` from `ctx` automatically.

## What this package does NOT do

- Consume events. That is `core/rabbitmq.Consumer` plus the
  consumer-specific handler.
- Persist events to PG. The PG inventory mirror is a separate
  capability.
- Buffer between producer and broker. A broker outage surfaces as
  a `Publish` error to the caller.
