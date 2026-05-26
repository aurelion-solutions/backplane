# workload_lineage

Derives a workload's ownership chain back to the originating human, and
persists append-only snapshots of that chain for replay.

The resolver walks: workload → owning employment → person → all of that
person's employments, then classifies the chain *terminus*. It is pure
state-derivation over inventory rows — no workflow, no events. The
owner-chain is what lets a posture policy ask "is this workload owned by
a terminated human?" without a graph engine.

## Terminus

The end of a resolved chain is one of:

- `active_human` — ends at a human with at least one employment active
  as of the resolve time.
- `terminated_human` — ends at a human all of whose employments have
  ended.
- `unowned` — the workload has no `owner_employment_id`.
- `broken_link` — an owner reference exists but the referenced row could
  not be found (a data-integrity gap).

"Terminated" is derived purely from `Employment` date windows
(`IsActiveAt`), because employment status is free-text `code` with no
closed enum.

## Shape

`OwnershipChain(workload_id, links[], terminus, resolved_at)` where each
`ChainLink(kind, ref_id, label?, terminated, end_date?)` is one step
(`kind` ∈ `workload` | `employment` | `person`).

`WorkloadLineageSnapshot(id, workload_id, resolved_at, terminus,
chain jsonb, chain_hash, created_at)` is the persisted row, idempotent on
`(workload_id, chain_hash)`.

- `chain_hash` — deterministic hex-SHA256 over the chain identity,
  **excluding** `resolved_at` (a wall-clock would defeat idempotency).

## Dependencies

Reads workloads / employments / persons through narrow reader ports
(`WorkloadReader`, `EmploymentReader`, `PersonReader`) returning
slice-local `*Ref` types — no import of those slices' internals, so the
dependency direction stays one-way.

## REST surface

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v0/workloads/:id/lineage` | Resolve and return the current ownership chain. Read-only — **never writes a snapshot**. |

## What this package does NOT do

- Write snapshots on the GET path. Snapshot writes happen exclusively in
  the `policy_assessment` assess pass (`cmd/worker`), which resolves
  every chain anyway.
- Emit findings. The assess workload pass turns a `terminated_human` terminus
  into a finding; this slice only resolves and stores the chain.
- Traverse a graph. It is a single deterministic owner walk, not the
  access-graph / `graph_analysis` substrate.
