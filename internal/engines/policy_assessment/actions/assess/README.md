# policy_assessment.assess

The orchestrator-registerable unit of work behind a scan run. One
invocation walks the configured account population through every
applicable policy and writes findings, gated by the deduplication
hash.

Pair: `("policy_assessment", "assess")`.

## Inputs (`Args`)

| Field | Required | Meaning |
|---|---|---|
| `triggered_by` | optional | Audit metadata on the assessment-run row. Defaults to `schedule`. |
| `application_id` | optional | Restrict the account population to one application. Empty → all applications. |
| `mechanisms` | optional | Allowlist of mechanism handlers to run. Empty → every mechanism the dispatcher knows about. |
| `created_by` | optional | Audit metadata — operator id when triggered through the UI. |

## Output (`Result`)

```go
type Result struct {
    AssessmentRunID   string
    AccountsEvaluated int
    PoliciesApplied   int
    Matched           int
    FindingsCreated   int
    FindingsReused    int
}
```

## What it does

1. Insert one row into `policy_assessment_runs` with
   `status = running`, anchor it to the optional scope filters.
2. List the account population from `accounts` via the action's
   `accounts.Repository.List(ctx, tx, ListFilter)` (paged snapshot).
3. For each account:
   - Build `Facts` (`Target{kind:"account"}` + `Resource` + scoped
     context).
   - Derive the request facet set, call `Store.SelectByFacets` to
     get applicable policies, and dispatch each entry through the
     mechanism handler.
   - When the handler returns a non-nil `Decision`, build a
     `findings` row from the mechanism output.
4. Persist findings with the canonical evidence-hash key. Duplicate
   inserts are caught at the DB layer and counted as
   `FindingsReused` instead of `FindingsCreated`.
5. Stamp the run row with `status = completed | failed`, attach the
   counters, and return.

## Idempotency

The finding `evidence_hash` is computed over canonical input —
policy id + principal/account anchors + scope + decision content.
The unique constraint on the findings table is `UNIQUE NULLS NOT
DISTINCT` over `(kind, principal_id, account_id, policy_id,
scope_key_id, scope_value, evidence_hash)`. Re-running the same
assess on the same population does not duplicate findings; it bumps
`FindingsReused`.

## Failure semantics

- Per-mechanism / per-policy errors are logged and skipped — the run
  proceeds.
- Repository or transaction errors abort the run with
  `status = failed`. Findings written before the abort stay
  committed; the next run reuses them.

## Composition

`cmd/worker` wires the action via
`assess.Register(reg, Deps{DB, AccountsRepo, RunsRepo, FindingsRepo,
Store, Dispatcher})`. The dispatcher and store are shared across
workers; the action only sees the repositories and the dispatcher
interface.
