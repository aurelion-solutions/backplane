# findings

One row per detected violation or anomaly emitted by a policy
assessment pass.

A finding belongs to exactly one
[assessment run](../policy_assessment_runs/) and references at
least one of `(principal, account)` — orphan-account-style findings
carry an account anchor without a principal. The policy that produced
the finding is referenced via `policy_id` when available; anonymous
emissions can omit it.

## Shape

`Finding(id, run_id, kind, severity, status, principal_id?,
account_id?, policy_id?, evidence_hash, active_mitigation_id?,
proposed_mitigation_id?, payload, ...)`.

- `kind` — free-form short label owned by the emitting policy
  (`orphan_access`, `terminated_access`, `sod`, …). Open vocabulary —
  new kinds arrive through new cartridges without a schema change.
- `severity` — `critical` / `high` / `medium` / `low`. CHECK constraint.
- `status` — `open` / `acknowledged` / `resolved` / `mitigated`. CHECK constraint.
- `evidence_hash` — canonical idempotency key. A re-emission of the
  same finding by the same policy against the same anchors reuses
  the existing row instead of duplicating.

## Mitigation columns

`active_mitigation_id` and `proposed_mitigation_id` are plain UUID
columns without a foreign key. The mitigations slice will add the
constraint when it ships.

## What this package does NOT do

- Run policies. Detection lives in
  `engines/policy_assessment/mechanisms/*`. This slice persists the
  output.
- Close findings automatically. Mitigation lifecycle is a separate
  workflow; `status` is operator-driven today.
- Define the kind vocabulary. Policies own their own labels.
