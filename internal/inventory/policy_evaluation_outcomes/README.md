# policy_evaluation_outcomes

One row per `(policy, target)` evaluation in an
[assessment run](../policy_assessment_runs/), recording its **ternary**
result.

[findings](../findings/) records only matched violations. PEO is the
superset that also records `not_matched` (clean) and `not_evaluable` —
the Blind Spots, where a rule could not be evaluated because a required
truth input was absent. `not_evaluable` is a first-class product output,
not an error: it lets a consumer surface "we couldn't check this, and
here's what's missing."

## Shape

`PolicyEvaluationOutcome(id, assessment_run_id, cartridge_id, rule_id,
target_type, target_ref?, target_key, outcome, missing_evidence jsonb,
source_id?, evaluated_at)`.

- `outcome` — `matched` / `not_matched` / `not_evaluable`. Closed set,
  DB CHECK.
- `target_type` — `account` / `subject` / `workload` are concrete entities
  (`target_ref` set); `source` / `pipeline` are aggregate coverage
  targets (`target_ref` nil, identity via `target_key`). Closed set,
  DB CHECK.
- `missing_evidence` — empty for `matched` / `not_matched`; lists the
  absent truth-input keys for `not_evaluable`.

## Invariants

- **The biconditional:** `outcome = not_evaluable` if and only if
  `missing_evidence` is non-empty. `RecordOutcome` enforces it.
- **Identity:** `(assessment_run_id, cartridge_id, rule_id, target_type,
  target_ref)`. Re-emission within the same run upserts rather than
  duplicating.

## REST surface

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v0/policy-evaluation-outcomes` | List, paginated (`limit`/`offset`). Filters: `assessment_run_id`, `outcome`, `cartridge_id`, `target_type`. |
| GET | `/api/v0/policy-evaluation-outcomes/:id` | Fetch one outcome. |

Outcomes are **written** by the worker policy-assessment action.

## What this package does NOT do

- Run policies. The `policy_assessment` mechanisms produce the verdicts;
  this slice persists them.
- Derive an `evidence_gap` finding from a `not_evaluable` outcome — that
  is the assess action's job, keeping PEO free of a sibling `findings`
  dependency.
