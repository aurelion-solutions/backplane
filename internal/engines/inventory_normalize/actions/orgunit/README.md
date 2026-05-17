# normalize.orgunit

Action for `dataset_type = orgunit`. Upserts organisation-unit nodes
from the lake into the `org_units` table and stitches the parent
hierarchy together.

## Why

Org units carry the structural attachment used by downstream rules:
inheritance rules in `access_generate` match a principal's OU against
`source_org_unit_dn`. A correct hierarchy in PG is the precondition.

## Natural key

`external_id` (provider-issued). Two records with the same
`external_id` are the same node — we UPDATE. Renames keep the row.

## Input shape

The action accepts two record shapes side-by-side, decided per
record by what the payload contains:

- **Tree shape** — one root node per record, recursive `children: [...]`.
  Typical of AD-style dumps. Each level carries
  `identifier`, `name` and the recursive child list.
- **Flat shape** — one node per record with
  `{external_id, name, parent_external_id?}`. Typical of CSV
  imports.

## Algorithm

1. Read `lake/orgunit/*.jsonl` for the given `lake_ref`.
2. For each record:
   - **Tree**: recurse, upsert each node while threading the
     parent's just-resolved id down to children.
   - **Flat**: collect all nodes in a first pass with `parent_id=NULL`,
     then a second pass resolves `parent_external_id` against the
     batch (in-memory map) and falls back to a PG lookup for
     cross-batch parents. Patch `parent_id` only when resolved.
3. Counters land on `Result`: `read` (root subtrees in the batch),
   `upserted` (total nodes touched), `skipped` (malformed payloads).

## Lake record shape

Tree:

```json
{
  "external_id": "ou-eng",
  "payload": {
    "identifier": "ou-eng",
    "name": "Engineering",
    "children": [
      { "identifier": "ou-eng-platform", "name": "Platform", "children": [] }
    ]
  }
}
```

Flat:

```json
{
  "external_id": "ou-eng-platform",
  "payload": {
    "external_id":        "ou-eng-platform",
    "name":               "Platform",
    "parent_external_id": "ou-eng"
  }
}
```

## What it does NOT do

- Resolve principals — that is a separate engine.
- Move principals between OUs — `employments.org_unit_id` is owned
  by `normalize.employee`.
- Soft-delete OUs whose nodes disappear from the source — the
  current revision is upsert-only; reconciliation lives elsewhere.
