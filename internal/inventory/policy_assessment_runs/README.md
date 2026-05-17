# policy_assessment_runs

One row per policy-assessment pass — the lifecycle ledger for a
single sweep over the policy catalogue.

A run records who triggered it, what scope it covered, when it
started and finished, and the counters (total findings, per-severity
breakdown, created vs. reused).

## Shape

`AssessmentRun(id, status, triggered_by, scope_principal_id?,
scope_application_id?, started_at, finished_at?, total_findings,
critical, high, medium, low, created, reused, error_message?, ...)`.

- `status` — `pending` / `running` / `completed` / `failed`. CHECK constraint.
- `triggered_by` — `manual` / `api` / `schedule`. CHECK constraint.
- Counters are populated as the run progresses; consumers should
  read counters only on `completed` rows.
- `scope_*_id` columns are optional: NULL → full sweep.

## Relationship with findings

[Findings](../findings/) reference the run via FK. The run cannot be
deleted while findings still point at it (`ON DELETE RESTRICT`).
Operators clean up by archiving the findings first or by leaving
the run in place — it's the audit ledger.

## Trigger paths

- Manual button in Lens → `POST /api/v0/assessments` (sync API entry)
- Worker pipeline `policy_assessment` → schedule / matcher
- Cartridge-driven schedule via the orchestrator beat

All three converge on the same `assess` action under
`engines/policy_assessment/actions/assess/`.

## What this package does NOT do

- Score the run itself. It's a ledger row, not a verdict.
- Emit notifications. That's downstream (`notifications` engine when
  it ships).
