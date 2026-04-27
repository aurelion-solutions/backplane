# persons

Canonical human identity. One Person has an `external_id` (stable
source-side id), `full_name`, and an EAV-style `person_attributes`
sidecar for arbitrary key/value annotations propagated from raw
records via the active mapping rules.

One Person aggregates one or more Employments (see
[`employments`](../employments/)) — a single record from the source
side delivers both the Person and its Employments inline.

## Ingest contract — `dataset_type = employee`

One lake record per Person, with all their Employments inlined.
Idempotent on the lake side: a re-delivery of unchanged content
hashes to the same value and is skipped before normalize ever runs.

```json
{
  "external_id": "p-001",
  "id":          "p-001",
  "full_name":   "Ivan Petrov",
  "ssn":         "123-45-6789",
  "employments": [
    {
      "start_date":    "2018-06-01",
      "end_date":      "2020-01-14",
      "org_unit_id":   "ou-qa",
      "org_unit_name": "QA Department",
      "title_id":      "title-qa-eng",
      "title_name":    "QA Engineer"
    },
    {
      "start_date":    "2020-01-15",
      "end_date":      null,
      "org_unit_id":   "ou-engineering",
      "org_unit_name": "Engineering Department",
      "title_id":      null,
      "title_name":    "Senior Developer"
    }
  ]
}
```

- `external_id` (top-level, required) — connector's stable id for the
  Person. Used for lake dedup and as the resolver-side determinator
  fallback.
- `id` — same value at payload level, for convenience when reading
  directly from the lake without unwrapping the envelope.
- `full_name`, `ssn` — Person-level fields. `ssn` is the canonical
  cross-source matching key (one human, one SSN, across HRIS / AD /
  SAP).
- `employments[]` — one entry per work period:
  - `start_date` (ISO date, required), `end_date` (ISO date or
    `null` for currently active),
  - `org_unit_id` / `org_unit_name` — identifier and/or human-readable
    name of the org unit. The source may send either or both (some
    HRIS systems publish only the id, some only the name). Normalize
    resolves to a row in [`org_units`](../org_units/): try
    `org_unit_id` first (stable key), fall back to `org_unit_name`.
    At least one of the two must be non-empty.
  - `title_id` / `title_name` — same idea for the job title: either
    or both. At least one must be non-empty.

Normalized by [`inventory_normalize.employee`](../../engines/inventory_normalize/actions/employee/).
