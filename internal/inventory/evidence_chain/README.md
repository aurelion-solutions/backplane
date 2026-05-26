# evidence_chain

Append-only lineage records linking an outcome or finding back through
the truth stack — raw row → normalised fact → effective grant →
justifying initiative — to the policy that produced it.

Each row is the "why does this result exist?" trail for one finding or
[policy-evaluation outcome](../policy_evaluation_outcomes/), anchored to
the [assessment run](../policy_assessment_runs/) that emitted it. That
anchor gives evidence an immutable, timestamped reference — the temporal
foundation period queries build on without retrofitting the shape.

## Shape

`EvidenceChain(id, scan_run_id, finding_id?, outcome_id?,
ingest_batch_id?, raw_row_hash?, normalized_kind?, normalized_id?,
capability_grant_id?, initiative_id?, policy_ref, chain_hash,
created_at)`.

- All lineage references are nullable — a chain records whichever truth
  layers actually exist for the outcome it explains. `scan_run_id`,
  `policy_ref`, and `chain_hash` are always set.
- `normalized_kind` — closed set, pinned by a DB CHECK constraint.
- `chain_hash` — deterministic SHA-256 over the component ids. It is the
  idempotency key: `RecordChain` is idempotent on it, so re-recording
  the same chain returns the existing row instead of duplicating.

## REST surface

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v0/evidence-chains/:id` | Fetch one chain. Read-only. |

Chains are **written** by the worker policy-assessment action, never
over HTTP.

## What this package does NOT do

- Run policies or compute the chain. The `policy_assessment` assess pass
  assembles the component ids; this slice persists them.
- Mutate. Rows are immutable — never updated in place.
- Derive findings from a chain. A chain explains an existing finding or
  outcome; it does not create one.
