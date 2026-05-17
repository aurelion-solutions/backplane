# access_generate

Computes the set of initiatives a principal *should* hold right now
by projecting structural state (employment, OU), ITSM requests, and
delegations through cartridge rules.

## Single entry point

```go
result, err := engine.Recompute(ctx, principalID, RecomputeFilter{...})
```

Every trigger reduces to this call:

- Journey pipeline action on `employee.org_unit_changed` etc.
- Beat-scheduled passes (daily reconcile, grace-period expiry checks)
- Ad-hoc REST trigger ("review principal X")

`RecomputeFilter` lets a trigger narrow the scope — "review only this
application", "review only this capability". When empty, the whole
(principal, ∀ applications, ∀ capabilities) scope is rebuilt.

## Pipeline inside Recompute

1. **collect planned** — fan-in from three sources:
   - `inheritance` — cartridge rules + Employment/OU
   - `requested` — ITSM Gateway *(stub, see `requested.go`)*
   - `delegated` — ITSM Gateway *(stub, see `delegated.go`)*
2. **apply filter** — drop planned entries outside the
   `RecomputeFilter` scope before the diff.
3. **load current** — read principal's currently-active initiatives
   from the repository within the same filter scope.
4. **diff** — compute toCreate / toTombstone.
5. **persist** — Create new rows, Tombstone old rows; both inside
   one transaction.
6. **emit MQ** — `inventory.initiative.created` /
   `inventory.initiative.tombstoned`, published after commit so a
   broker outage cannot leave subscribers ahead of the database.

The desired-state recompute on accounts/grants is not in this engine;
it is a separate concern downstream of the initiative diff.

## Rule loading

Rules come from the cartridge bundle named in `Deps.BundleRef`. We
call `Cartridges.Provider.Policies(ref)`, filter by
`Mechanism == "inheritance"`, and parse each `Manifest.Body`. The
`cartridges` core package owns the question of "where do these files
actually live"; this engine never walks the filesystem directly.

## Inheritance rule body shape

```json
{
  "source_org_unit_dn": "corp/europe/engineering",
  "grants": [
    { "application_slug": "microsoft_ad" },
    { "application_slug": "github", "capability_slug": "developer" }
  ]
}
```

- `source_org_unit_dn` — slash-joined OrgUnit `name` path from root
  to leaf. v1 matches exactly; prefix-match is a future extension.
- `grants` — what a principal in this OU inherits.
   - `application_slug` alone → account-initiative ("must have an
     account in this app").
   - `application_slug` + `capability_slug` → grant-initiative.

## What gets written

| Column | Source |
|---|---|
| `Initiative.Actor` | `<engine actor>:<source_rule_id>` so the audit trail says which exact rule fired |
| `Initiative.Justification` | source-specific blob — for inheritance: `{"source_rule_id": "...", "source_org_unit_dn": "..."}` |
| `Initiative.Kind` | `inheritance`, `requested`, or `delegated` |
| `Initiative.ValidFrom / ValidUntil` | match the source's own window (request validity, delegation window); inheritance is open-ended |

## Diff key

Two initiatives are "the same logical row" when they share
`(kind, application_id, capability_id, source_rule_id)`. `source_rule_id`
travels in `Justification`. Two different rules issuing accidentally
the same (kind, application, capability) produce two separate
initiatives — that's the desired behaviour: access ⇐ any single
active justification.

## Stubs (intentional)

`requested.go` and `delegated.go` are stubs with detailed comments
covering the expected shape once the ITSM Gateway lands. They return
nil so the engine works today on inheritance alone.

## Not in this package

- `validated_state` writes — that's the future `access_validate`
  (PDP) engine.
- `effective_state` writes — `inventory_normalize` (from connector
  data) and the future `access_promote` (when it ships a command and
  parks effective at `pending`).
- MQ subscription / dispatch — Journey-side pipelines translate
  domain events into `Recompute` calls; this engine never subscribes
  directly.
