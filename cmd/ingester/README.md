# ingester

Lake-stream worker. Consumes `aurelion.ingest` one message per
record, windows incoming records per `(source, dataset_type,
correlation_id)`, runs the DuckDB anti-join against the current lake
state, and writes only new / changed records into the lake. Emits
`inventory.ingest.batch_received` after each window commits.

## What this binary IS

- The only writer to the lake (`storage/file`, future `s3` /
  `iceberg`). Every other component reads.
- The dedupe gate. The anti-join is what makes the lake additive
  but not duplicative across replayed connector batches.
- Horizontally scalable. Run N replicas; the durable
  `aurelion.ingest` queue shards deliveries across them.

## What this binary IS NOT

- A normaliser. `inventory_normalize.*` actions read the lake; they
  do not run here.
- A connector. Connectors publish `aurelion.ingest`; this process
  consumes it.
- An HTTP server. There is no operator surface — observability is
  log lines + the published events.

## Required env

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `AURELION_INGESTER_INSTANCE_ID` | yes | — | Unique per replica; surfaced in logs and as the AMQP consumer tag. Boot fails without it. |
| `AURELION_SECRET_PROVIDER` | no | `file` | Secret backend selector. |
| `AURELION_SECRETS_FILE` | no | `.secrets.json` | File-backend path. |

Everything else (PG DSN, MQ URL, exchange names, lake root) comes
from the secret manager via `core/config`.

## Multi-replica

Replicas compete for the same durable queue; AMQP sharding takes care
of delivery distribution. The `correlation_id` window is per-message
local — two replicas never see the same record, so the anti-join
stays correct.
