# rabbitmq

Connection lifecycle, exchange declaration, and the basic Consumer /
RPC client built on top of `streadway/amqp`-style API. Pure
infrastructure — every component that publishes or consumes events
goes through here.

## Pieces

| File | Role |
|---|---|
| `rabbitmq.go` | `Dial(Config)` → connection, exchange declarations |
| `consumer.go` | `Consumer` — queue bind + delivery loop with handler callback |
| `rpc_client.go` | `RPCClient` — request/response over reply-to queues |

## Exchange declaration

Each exchange declared from `Config.Exchanges` carries its own type
(topic / direct / fanout / headers). Type mismatches surface as
PRECONDITION_FAILED at boot — desired behaviour: an existing
exchange with the wrong type is a configuration drift the operator
must resolve.

Declaration is idempotent: re-declaring with the same name + type +
durability is a no-op. There is no "redeclare on every boot" cost.

## Consumer

```go
c, err := rabbitmq.NewConsumer(conn, rabbitmq.ConsumerConfig{
    Queue:        "journey.intake",
    Exchange:     "aurelion.events",
    RoutingKeys:  []string{"inventory.employee.updated"},
    PrefetchCount: 32,
    Handler: func(ctx context.Context, msg amqp.Delivery) error { ... },
})
```

- Queue and bindings are declared idempotently on start.
- Delivery ack is positive on `nil`, `nack(requeue=false)` on
  handler error — the dead-letter is the caller's responsibility
  to wire if needed.
- The handler receives the raw delivery; envelope parsing belongs
  to the consumer-specific code (`core/events.Envelope`).

## RPC client

Thin request/response over `reply_to` + `correlation_id`. Used by
connectors that want a synchronous answer from the backplane (or
vice versa). Not the right tool when the answer is large or slow —
prefer a regular publish-and-consume there.

## What this package does NOT do

- Define event shapes — see `core/events.Envelope`.
- Persist messages — the broker owns durability.
- Implement retries or exponential backoff beyond what the AMQP
  client offers natively.
