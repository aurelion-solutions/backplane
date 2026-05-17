# core/pipelines

Sync side of the cartridge → PG pipeline mirror.

`inventory/pipelines` owns the table; this package owns the loop that
keeps it in line with the cartridge filesystem.

## What lives here

| File | Role |
|---|---|
| `sync.go` | `Manager` + `Sync(ctx)` (one reconciliation pass) + `RunSyncLoop(ctx, db, interval)` (advisory-locked tick loop). |
| `sync_test.go` | Insert / no-op / soft-delete / bad-YAML behaviour. |

## Sync semantics

For every cartridge served by the provider:

1. `paths  = provider.Pipelines(ref)`
2. `active = repo.ListActiveByCartridge(ref)`
3. For each `path` the orchestrator loader parses the YAML and
   computes a canonical content hash. The result is upserted into the
   pipelines table (insert / update / resurrect handled in one
   `ON CONFLICT ... DO UPDATE` statement).
4. For each PG-active row whose name no longer appears in the cartridge,
   `MarkRemoved`.

A YAML file that fails to load is logged and skipped — corrupt files
do not knock out the rest of the cartridge.

## Cluster-wide singleton

`RunSyncLoop` wraps each tick in `pg_try_advisory_lock(AURELPIP)` —
same pattern as `core/policies` and `orchestrator/beat`.

## Interval

Default 5 s. Override via wiring when needed.
