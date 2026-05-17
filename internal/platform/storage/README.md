# storage

Data-lake batch storage. Connector output lands here as ordered
batches keyed by an opaque `storage_key`, and the lake is the
source-of-truth for everything ingest / normalize / import does
downstream.

## Contract

```go
type Storage interface {
    WriteBatch(ctx, datasetType string, records []map[string]any) (storageKey string, err error)
    ReadBatch(ctx, storageKey string) ([]map[string]any, error)
    DeleteBatch(ctx, storageKey string) error
    AntiJoin(ctx, datasetType string, candidates []Candidate) (AntiJoinResult, error)
}
```

Records carry a fixed three-key shape: `external_id`, `meta`
(backplane-added: `hash`, `committed_at`, `correlation_id`), and
`payload` (verbatim from the source).

## Anti-join

`AntiJoin` is the dedupe gate. Given a set of `(external_id, hash)`
pairs and a `datasetType`, it returns:

- `NewIDs` — never seen in the lake before
- `ChangedIDs` — present but with a different latest hash

Anything in the input list that's missing from both is unchanged and
should be skipped. The anti-join is read-only — it does not mutate
the lake. The ingester runs it before every commit so replayed
connector batches don't bloat the lake with duplicates.

## Providers

| Name | File | Status |
|---|---|---|
| `file` | `file.go`, `file_antijoin.go` | wired — local filesystem, DuckDB for anti-join |
| `s3` | `s3.go` | stub |
| `iceberg` | `iceberg.go` | stub |

Stubs embed `Stub{}` — every method returns `ErrNotImplemented`.

## Factory

```go
sf := storage.NewFactory()
storage.RegisterFile(sf, "/var/lib/aurelion/lake")
storage.RegisterS3(sf)
storage.RegisterIceberg(sf)
lake, err := sf.Get(settings.Storage.Provider)
```

Safe for concurrent use.

## Errors

- `ErrNotFound` — `storage_key` does not exist in the backend.
- `ErrNotImplemented` — stub provider.
- Transport / IO errors propagate as-is.

## What this package does NOT do

- Normalise. The ingester writes raw `payload` as the source sent
  it; engines under `internal/engines/inventory_normalize/*` reshape
  it into business entities on read.
- Iterate lazily. The contract returns whole batches in memory; for
  very large batches switch to `iter.Seq` here and update all callers.
- Encrypt. Encryption is a property of the backend (S3 SSE-KMS,
  Iceberg table props), not of this package.
