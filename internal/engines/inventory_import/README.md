# inventory_import

Synchronous CSV-import façade. Runs ingest + the matching normalize
action against the same input inside a single HTTP request.

## Endpoint

```
POST /api/v0/inventory/import
```

Request:

```json
{
  "source":         "lens.csv",
  "dataset_type":   "employee",
  "correlation_id": "...",
  "records":        [{ "external_id": "...", "...": "..." }]
}
```

Response (200):

```json
{
  "ingest": {
    "batch_id":    "...",
    "received":    1042,
    "written":     1042,
    "new":         800,
    "changed":     242,
    "skipped":     0,
    "lake_ref":    "..."
  },
  "normalize": {
    "...": "shape depends on dataset_type"
  }
}
```

Status codes: 200 on success, 400 on envelope validation failures
(bad dataset_type, empty records, missing external_id, batch too
large), 500 on anything else.

## Sequencing

1. `inventory_ingest.Process` with `SkipEvent=true` — writes the
   lake + audit row, suppresses the MQ `inventory.ingest.batch_received`.
2. Look up the normalize action for `dataset_type` against the
   internal whitelist. Unknown values are rejected.
3. Open one `bun.DB.RunInTx` and dispatch the action with `batch_id`,
   `source`, `lake_ref`. The action runs all its writes against this
   transaction; failure rolls back every PG write.

## Supported dataset_types

| dataset_type           | normalize action                                |
|---|---|
| `employee`             | `inventory_normalize.employee`                  |
| `account`              | `inventory_normalize.account`                   |
| `orgunit`              | `inventory_normalize.orgunit`                   |
| `access_grant_record`  | `inventory_normalize.access_grant_record`       |

Adding a new dataset = one line in `datasetActions` in
`service.go`. No other moving parts in this package.

## Why this exists

The async path (`/ingest` → MQ → normalize-action) is the right
answer for connector traffic — decoupled, retryable, observable
through the pipeline catalog. The Lens CSV demo wizard has a
different requirement: one request, one response, with a clear
"rows imported and normalized" answer in the body. This package is
the explicit synchronous path for that case.
