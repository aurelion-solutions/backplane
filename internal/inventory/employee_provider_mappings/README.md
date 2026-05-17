# employee_provider_mappings

Per-provider rules used by `inventory_normalize.employee` to resolve
raw [EmployeeRecords](../employee_records/) into canonical
[Persons](../persons/) via the determinator + upstream-attach
pattern.

One row per `(provider, record_key)`. Admin-managed, pure reference
data.

## Shape

`Mapping(id, provider, record_key, person_key, is_determinator,
allow_upstream, is_active, ...)`.

- `provider` — source name (e.g. `hris`, `ad`, `sap`).
- `record_key` — attribute on the raw record (e.g. `employee_id`,
  `email`, `sam_account_name`).
- `person_key` — canonical attribute key on Person the value lands
  on.
- `is_determinator` — TRUE: this provider is authoritative for
  Person identity. The action MAY create a new Person when no match
  exists.
- `allow_upstream` — TRUE: this row may attach a record to an
  EXISTING Person via attribute lookup but never creates one.

`is_determinator` and `allow_upstream` are not mutually exclusive: a
provider can both create and attach.

## Resolver behaviour (driven by these rows)

```
1. For each record: try determinator rules first — if Person exists,
   attach; else if rule allows, CREATE.
2. Then try upstream rules — attach only.
3. Records with no matching mapping land in employment_record_matches
   as unresolved, surfaced by an inventory.employee_record event.
```

See [`employee_records`](../employee_records/) resolver.go for the
runtime side.

## What this package does NOT do

- Hold the matched lineage rows. Those live in
  [`employment_record_matches`](../employment_record_matches/).
- Validate provider names against a closed set — providers are
  free-form strings keyed by what connectors publish.
