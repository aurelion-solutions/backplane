# org_units

Tree of organisational nodes (companies / departments / teams /
project seats). Two parallel hierarchies coexist:

- **internal** (`is_internal = true`) — backplane-managed, seeded by
  deployment;
- **external** (`is_internal = false`) — synced from source systems
  via the orgunit contract below.

Employments, profiles, and many access primitives reference OrgUnit
to express "which unit does this belong to".

## Ingest contract — `dataset_type = orgunit`

One record per **subtree**, sent recursively from a root. The lake
hashes the whole tree — any change at any depth re-emits a new
revision. New / removed nodes appear as additions / disappearances
under `children`.

```json
{
  "external_id":  "ou-root",
  "identifier":   "ou-root",
  "name":         "company",
  "display_name": "Acme Corp",
  "is_active":    true,
  "label":        null,
  "company":      null,
  "manager_id":      null,
  "meta":         null,
  "children": [
    {
      "identifier":   "ou-finance",
      "name":         "finance",
      "display_name": "Finance Department",
      "is_active":    true,
      "label":        "Finance",
      "manager_id":      "p-010",
      "children": [
        {
          "identifier":   "ou-accounting",
          "name":         "accounting",
          "display_name": "Accounting",
          "is_active":    true,
          "manager_id":      "p-020",
          "children":     []
        }
      ]
    },
    {
      "identifier":   "ou-engineering",
      "name":         "engineering",
      "display_name": "Engineering Department",
      "is_active":    true,
      "manager_id":      "p-007",
      "children":     []
    }
  ]
}
```

- `external_id` (top-level, required) — stable id of the subtree root.
- `identifier` (required) — same value at payload level; nested nodes
  use it as their own stable key. Employments reference OrgUnit by
  `name` / `identifier` (the choice is up to the normalize-side
  mapping).
- `name` (required) — machine-friendly stable code.
- `display_name` (required) — human-friendly label for UI / reports.
- `is_active` (default `true`) — soft-deactivate when the source
  marks the unit as retired. Active children under a retired parent
  are tolerated during migrations.
- `label` — short tag / alias.
- `company` — reference to a parent legal entity (string identifier).
- `manager_id` — Person identifier of the unit's manager (matches
  `external_id` in [`persons`](../persons/)).
- `meta` — open-ended JSON (headcount, geo, custom fields).
- `children[]` — nested OrgUnits, same shape recursively. Empty
  array / missing field = leaf.

Normalized by `inventory_normalize.orgunit` (not yet implemented)
into `org_units` and the corresponding hierarchy mirror.

## Source of truth

The contract mirrors the `OrgUnitDTO` shape from a sister project
almost 1:1: identifier, name, display_name, is_active, label,
children, company, manager_id, meta. The only adaptations are that
`external_id` is duplicated onto the wrapper top level to fit the
ingest envelope, and `manager` is renamed to `manager_id` to follow
Aurelion's `_id` convention for FK-like references.
