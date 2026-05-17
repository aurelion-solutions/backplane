# policies

Inventory slice for the PG mirror of cartridge-defined policy rules.

## What lives here

| File | Role |
|---|---|
| `model.go` | `Policy` row (bun ORM). |
| `repository.go` | `Repository` interface + `BunRepository`. Upsert / MarkRemoved / Resurrect on the sync side; List / GetByID / GetByNaturalKey / ListActiveByCartridge / ListActiveByMechanisms on the read side. |
| `routes.go` | `RegisterRoutes(g, repo)` — `GET /policies` (filter by `cartridge_ref`, `mechanism`, `include_inactive`), `GET /policies/:id`. |
| `errors.go` | `ErrNotFound`. |

## What this slice does NOT do

- **No mutations from HTTP.** Edits land via cartridge changes picked
  up by `core/policies` on the next sync tick.
- **No event emission.** Catalog churn does not produce inventory
  events (no consumer asked for them; the 5-second sync interval is
  the contract).
- **No Rego bodies.** The body lives in the cartridge `.rego` file
  and is loaded directly by consumer engines (PDP, scan engine).
  This table holds metadata only.

## Soft-delete semantics

When a cartridge no longer ships a rule that PG knows about, the sync
loop sets `is_active=false` and stamps `removed_at`. The row stays so
that findings already in flight can still reference the rule by id.
If the cartridge later brings the rule back, Upsert resurrects the
existing row in place — preserving the id.

## Natural key

`(cartridge_ref, rule_id)`. Public display form is
`<cartridge_ref>/<rule_id>`. UUIDs stay internal — consumers join
through the natural key when they have it from the cartridge.
