# pipelines

Inventory slice for the PG mirror of cartridge-defined pipeline
definitions.

## What lives here

| File | Role |
|---|---|
| `model.go` | `Pipeline` row (bun ORM). |
| `repository.go` | `Repository` interface + `BunRepository`. Upsert / MarkRemoved / Resurrect on the sync side; List / GetByID / GetByNaturalKey / ListActiveByCartridge on the read side. |
| `routes.go` | `RegisterRoutes(g, repo)` — `GET /pipelines` (filter by `cartridge_ref`, `include_inactive`), `GET /pipelines/:id`. |
| `errors.go` | `ErrNotFound`. |

## What this slice does NOT do

- **No mutations from HTTP.** Edits land via cartridge changes picked
  up by `core/pipelines` on the next sync tick.
- **No event emission.** Catalog churn does not produce inventory
  events.
- **No YAML body.** The runtime catalog (`orchestrator/loader`) still
  parses YAML straight from the cartridge filesystem. This table
  holds metadata + a content hash; the hash is what the sync loop
  diffs to decide if a row needs an Upsert.

## Natural key

`(cartridge_ref, name)`. Pipeline name is the loader-parsed
`pipeline.name` from the YAML — already globally unique across the
catalog at load time; namespacing by cartridge gives the projection a
stable owner column.
