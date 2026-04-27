# inventory_discover

Orchestrator for pull-side ingest. Where `inventory_ingest` waits to
be called, this engine actively asks a connector to start producing
records and tracks the resulting run until it completes.

## Why this engine exists

Some sources push (CSV uploads, AD webhooks, scheduled HRIS sync
from the connector side). Others have to be polled — the backplane
has to tell the connector "go discover and stream the result". This
engine owns that lifecycle.

It does **not** stream records itself. Records arrive at the lake
through the same `aurelion.ingest` MQ exchange that any push-side
connector uses: the discover-target connector publishes its records
there, stamped with the same `correlation_id` discover dispatched.
That keeps the lake-write path single-source — both push and pull
go through `inventory_ingest`.

## How it works

1. Caller posts `(connector_instance_id, operation, dataset_type)`.
2. The engine creates a `DiscoverRun` row with status `dispatched`
   and generates a `correlation_id` (the run's UUID).
3. It publishes one fire-and-forget command into the connector
   commands exchange. No RPC reply is awaited — the connector
   communicates asynchronously via MQ events.
4. As the connector works, it:
   - emits `connector.discover.started` → run flips to `running`,
   - publishes its records one-per-message into `aurelion.ingest`
     (stamped with the same `correlation_id`),
   - emits `connector.discover.completed` or
     `connector.discover.failed` → run flips to `completed` /
     `failed`.
5. A subscriber consumes those connector events and walks the run
   through its terminal state. `inventory.discover.run_completed`
   or `run_failed` is then emitted by this engine.

## Lifecycle

```
dispatched   ─►  running   ─►  completed
              │              │
              └────►  failed ─┘   (any stage)

(stuck in dispatched / running too long ─►  timed_out — sweeper, future)
```

`received_count` and `written_count` on the run row are populated
later by aggregating `inventory.ingest.batch_received` events that
carry the same `correlation_id`. They are eventually consistent —
the run can already be `completed` while the counts are still
catching up.

## Boundaries

This engine does **not**:

- write to the lake,
- buffer, validate, or shape records — connectors publish straight
  into `aurelion.ingest`,
- block on the connector's reply — dispatch is fire-and-forget,
- enforce scheduling — that belongs to an orchestrator pipeline
  cartridge that calls `Fetch` on a schedule.

## REST surface

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/v0/discover/runs` | Dispatch a discover command. 202 with the new run row, 400 on envelope errors, 502 if dispatch failed. |
| GET | `/api/v0/discover/runs` | List runs paginated by `started_at DESC`. |
| GET | `/api/v0/discover/runs/:id` | Fetch one run. |

## Events emitted (by this engine)

| Type | When | Payload |
|---|---|---|
| `inventory.discover.run_dispatched` | After Insert + successful dispatch. | `run_id`, `connector_instance_id`, `operation`, `dataset_type`, `status` |
| `inventory.discover.run_completed` | Connector signalled completion. | same + `status=completed` |
| `inventory.discover.run_failed` | Dispatch failed or connector signalled failure. | same + `status=failed`, `error` |

## Events listened (from connectors)

| Type | Effect |
|---|---|
| `connector.discover.started` | run.status: `dispatched` → `running` |
| `connector.discover.completed` | run.status: → `completed` |
| `connector.discover.failed` | run.status: → `failed`, error message captured |

Connectors must stamp the same `correlation_id` discover dispatched
to them so the subscriber can find the right run row.

## Dependencies

All via `Deps`:

- `CommandDispatch` — narrow port to publish the fire-and-forget
  command. Composition root wires an adapter over an AMQP channel.
- `core/events` — `EventSink`.
- `Repository` — Postgres-backed run-row store.
