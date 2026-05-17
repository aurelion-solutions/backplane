# normalize.person

Action for `dataset_type = person`. Minimal upsert of canonical
identities into the `persons` table.

## Why

A Person is the identity-graph anchor: every `Employment` row points
at a `Person`, the resolver in `normalize.employee` reaches for a
Person by `external_id`, and downstream lookups (e.g. case display
name in Journey) terminate on `Person.full_name`. The action exists
so a CSV / connector can land a flat list of people without going
through the heavier employee determinator.

## Natural key

`external_id` (provider-issued). Repeat records with the same
`external_id` update the existing row.

## Algorithm

1. Read `lake/person/*.jsonl` for the given `lake_ref`.
2. For each record:
   - Require `external_id` and `payload.full_name`; missing fields
     → `Skipped++`.
   - UPSERT into `persons` by `external_id`. New rows get a fresh
     UUID; existing rows have `full_name` overwritten.
3. Counters land on `Result`: `read`, `upserted`, `skipped`.

## Lake record shape

```json
{
  "external_id": "p-12345",
  "payload": {
    "full_name": "Alice Smith"
  }
}
```

Other payload fields are ignored on this revision — extension goes
into `person_attributes` (EAV) when the need lands.

## What it does NOT do

- Build `Employment` rows — that is `normalize.employee`.
- Resolve a Person across multiple provider records (`A. Smith` in
  HR vs `Alice Smith` in IT) — same `external_id` is the same
  Person, no fuzzy match.
- Write EAV attributes — left to a follow-up action when the
  payload schema grows.
