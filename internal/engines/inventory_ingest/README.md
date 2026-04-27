# inventory_ingest

The single writer into the data lake. A pure function: given
`(source, dataset_type, records)` it hashes each record, anti-joins
against the lake's latest revision per `external_id`, writes only the
new and changed records as one lake batch, persists an audit row,
and emits `inventory.ingest.batch_received`.

## Why this engine exists

A naive lake design — "every connector run writes every record" —
collapses fast. A single AD or HRIS feed easily holds millions of
records, but only a handful change per sync. Writing the rest every
time blows up lake size, downstream processing, and cost.

This engine is the deduplication boundary. It does no normalisation,
no entity resolution, no business logic. It only decides "did this
row really change?" and writes the minimum.

## How it works

1. Caller hands the engine a batch: `source`, `dataset_type`, and
   the raw records.
2. The engine validates that every record carries `external_id` —
   without it dedup is impossible.
3. Each record is hashed via canonical-JSON sha256.
4. The lake is asked: of these `(external_id, hash)` pairs, which
   are absent (`new`) and which differ from the current latest
   (`changed`)? This is one DuckDB anti-join over the JSONL lake
   files for the dataset.
5. Only the new+changed subset is shaped as
   `{external_id, meta, payload}` and written to the lake as one
   batch file. If nothing changed, no file is written.
6. An audit row is inserted (counts, lake_ref, correlation_id) and
   `inventory.ingest.batch_received` is emitted.

The lake row shape:

```json
{
  "external_id": "E001",
  "meta": {
    "hash":           "sha256...",
    "committed_at":   "2026-04-20T...",
    "correlation_id": "uuid-..."
  },
  "payload": {
    "external_id": "E001",
    "username":    "alice",
    "is_locked":   false
  }
}
```

`meta` is what backplane adds at write time; `payload` is what the
source sent verbatim. Hash is computed over `payload` (not over the
wrapper) so re-deliveries of identical content produce identical
hashes regardless of map iteration order.

## Transport boundaries

The engine owns no transport. It is consumed by two binaries:

| Process | Caller | Why |
|---|---|---|
| **`backplane`** | HTTP `POST /api/v0/ingest` | CLI / upstream callers with records already in memory, wanting a synchronous answer with counts. Backplane stays responsive because these batches are tiny. |
| **`ingester`** (separate process) | `transports/ingest_mq` AMQP consumer | Connectors publish one record per AMQP message; the consumer windows them per `(source, dataset_type, correlation_id)` and calls `Process` once per window. Lives in its own process so a million-record HRIS pull never starves backplane goroutines, and N replicas scale horizontally on the same durable queue. |

The discover engine does NOT call `Process` directly — when a pull
starts, the connector itself publishes records straight into the MQ
exchange and the ingester picks them up. Discover only orchestrates
the lifecycle.

## Boundaries

This engine does **not**:

- normalise, validate, or interpret payload fields beyond
  `external_id`,
- track per-record history outside the lake itself — that history is
  the lake,
- detect rows removed from the source (no full-snapshot semantics on
  ingest; removals are an upstream concern),
- chunk large batches across multiple lake files — one Process call
  → at most one lake file. `BatchLimit` caps a single call at
  50 000 records,
- decide what to do with the deduped records — that is
  normalize / reconcile / apply territory.

## REST surface

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/v0/ingest` | Process one batch synchronously. 201 with `{batch_id, received, written, skipped, new, changed, lake_ref}`. 400 on envelope or missing-external_id errors, 502 on lake errors. |
| GET | `/api/v0/ingest/batches` | List audit rows paginated by `completed_at DESC`. |
| GET | `/api/v0/ingest/batches/:id` | Fetch one audit row. |

`POST /ingest` also honours `X-Correlation-Id` HTTP header — if no
`correlation_id` is in the body, the header value is used.

## Events emitted

| Type | When | Payload |
|---|---|---|
| `inventory.ingest.batch_received` | Process completed (with or without lake writes). | `batch_id`, `source`, `dataset_type`, `received`, `written`, `skipped`, `new`, `changed`, `lake_ref?` |

Downstream (normalize / reconcile) subscribes to this event to know
that a fresh delta has landed.

## Dependencies

All via `Deps` at construction — no direct cross-engine imports:

- `Lake` — narrow port over `platform/storage` providing `AntiJoin`
  and `WriteBatch`.
- `core/events` — `EventSink`.
- `Repository` — Postgres-backed audit-row store.

## MQ wiring

The MQ consumer lives in `internal/transports/ingest_mq/`, not in
this package, because it is a transport — not a piece of business
logic. The `cmd/ingester` binary mounts that consumer at startup;
backplane does not. See [`transports/ingest_mq/doc.go`](../../transports/ingest_mq/doc.go)
for queue / exchange / window details and [`cmd/ingester/main.go`](../../../cmd/ingester/main.go)
for the deployment shape.

## Dataset_type contracts

The engine itself doesn't enforce per-`dataset_type` payload shape —
it only requires top-level `external_id`. The contracts live **next
to their inventory packages**: connectors target them, normalize
actions consume them. New dataset_types append to the table here.

| `dataset_type` | Contract | Normalized by |
|---|---|---|
| `employee` | [`inventory/persons/README.md`](../../inventory/persons/) | [`inventory_normalize.employee`](../inventory_normalize/actions/employee/) |
| `account` | [`inventory/accounts/README.md`](../../inventory/accounts/) | [`inventory_normalize.account`](../inventory_normalize/actions/account/) |
| `access_grant_record` | [`inventory/capability_grants/README.md`](../../inventory/capability_grants/) | [`inventory_normalize.access_grant_record`](../inventory_normalize/actions/access_grant_record/) |
| `access_artifact` | [`inventory/capability_grants/README.md`](../../inventory/capability_grants/) (read-only) | — (lake-only) |
| `orgunit` | [`inventory/org_units/README.md`](../../inventory/org_units/) | `inventory_normalize.orgunit` (TBD) |
