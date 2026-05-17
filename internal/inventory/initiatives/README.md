# initiatives

Audit trail behind every desired-state decision. An Initiative answers
the question "why does this principal need this account / capability
in this application?".

## Target shape

| `capability_id` | Initiative covers |
|---|---|
| `NULL`     | the existence of an account |
| `NOT NULL` | one capability on that account |

Multiple **active** initiatives may coexist for the same target —
access ⇐ any single active justification. No partial unique index
enforces "one active row per target".

## Validity window

`valid_from` and `valid_until` carry the **planned** window during
which the initiative is in force.

- Common case — both unset on insert: `valid_from` defaults to NOW()
  (column default + Go fallback in `Repository.Create`), `valid_until`
  stays NULL meaning open-ended.
- Scheduled case — "Monday onwards for two weeks": caller sets both
  explicitly. The initiative is inactive until `valid_from` arrives
  and deactivates again once `valid_until` passes.

`Initiative.IsActiveAt(t)` and `Initiative.IsActive()` combine the
window check with the tombstone check. `ListFilter.ActiveOnly` does
the same in SQL.

## Tombstone, never delete

Initiatives are audit records and **must not be removed**. Closure is
expressed by stamping `tombstoned_at`. No `closed_by`, no
`closure_reason` — the source of the justification (an org-unit
assignment, an approved request) is what gets revoked in its own
system; the platform reacts by tombstoning the initiative.

The `Repository` interface intentionally has no `Delete` method. Use
`Tombstone` to mark inactive, never DROP / DELETE.

## Repository

```go
Create(ctx,    tx, *Initiative) error
Tombstone(ctx, tx, id)          error   // idempotent
GetByID(ctx,   tx, id)          (*Initiative, error)
List(ctx,      tx, ListFilter)  ([]*Initiative, int, error)
```

`Tombstone` is idempotent — repeat calls on an already-tombstoned
row return nil. `ErrNotFound` only surfaces when the id does not
exist.

`ListFilter` supports filters on principal / application / capability
plus mutually-exclusive flags `AccountInits / GrantInits` and
`ActiveOnly / TombstonedOnly`, plus `Kind`.

## Kinds

The `kind` column carries one of:

| Value | Meaning |
|---|---|
| `inheritance`  | Derived from a structural attachment of the principal — org-unit membership today, project membership later. The specific source goes into `justification`. |
| `requested`    | Created through an approval workflow at the principal's request. |
| `delegated`    | Issued by another principal acting on the subject's behalf. The delegator goes into `justification`. |

Grace-period extensions are **not** a separate kind. They are
expressed as a follow-up initiative (of the original kind) with a
bounded `valid_until`, or as an existing row's `valid_until` being
pushed out.

The list is intentionally short and closed. New kinds require a
design discussion — adding them ad-hoc breaks the audit story.
