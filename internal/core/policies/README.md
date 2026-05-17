# core/policies

Sync side of the cartridge → PG policy mirror.

`inventory/policies` owns the table; this package owns the loop that
keeps it in line with the cartridge filesystem.

## What lives here

| File | Role |
|---|---|
| `sync.go` | `Manager` + `Sync(ctx)` (one reconciliation pass) + `RunSyncLoop(ctx, db, interval)` (advisory-locked tick loop). |
| `sync_test.go` | Insert / update / soft-delete / resurrect behaviour against in-memory stubs. |

## Sync semantics

For every cartridge served by the provider:

1. `manifests = provider.Policies(ref)`
2. `active   = repo.ListActiveByCartridge(ref)`
3. For each manifest, `Upsert` — handles INSERT, UPDATE, and resurrect
   of soft-deleted rows in one `ON CONFLICT ... DO UPDATE` statement.
4. For each PG-active row whose rule_id no longer appears in the
   manifest, `MarkRemoved` (soft-delete, metadata preserved).

The Rego body is never read here — only metadata. PDP / worker reload
the actual `.rego` files via their own `cartridges.Watcher`.

## Cluster-wide singleton

`RunSyncLoop` wraps each tick in a `pg_try_advisory_lock(AURELPOL)` —
same pattern as `orchestrator/beat`. N replicas of backplane all tick
on schedule; only one acquires the lock and reconciles. The others
log `Skipped` and exit the tick. Failover is implicit — whichever
replica grabs the lock on the next tick is the new singleton.

## Interval

Default 5 s, matching `cartridges.DefaultPollInterval`. Override per
deployment if the cartridge tree is large enough to make the scan
expensive (unlikely until tens of thousands of rules).
