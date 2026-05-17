# employee_records

External, source-side row representing a person as **one specific
application** sees them. The raw side of identity — before normalization
into canonical [Persons](../persons/) and
[Employments](../employments/).

One EmployeeRecord is unique per `(application_id, external_id)`. A
human typically has several — one per source system (HRIS, AD, SAP,
etc.).

## Shape

`EmployeeRecord(id, external_id, application_id, description?)` plus
an `EmployeeRecordAttribute(employee_record_id, key, value)` EAV
sidecar.

## Resolver

`resolver.go` glues EmployeeRecords to canonical entities using the
[`employee_provider_mappings`](../employee_provider_mappings/) rules:

1. Determinator pass — authoritative sources (HRIS-style) may CREATE
   a new Person when no match exists.
2. Upstream-attach pass — secondary sources (AD-style) may ATTACH
   their record to an EXISTING Person but never create.
3. Matched lineage lands in
   [`employment_record_matches`](../employment_record_matches/).

The resolver is called by `inventory_normalize.employee`; this slice
owns the data, the resolver code, and the unit tests for resolution
edge cases (multi-match collisions, upstream without prior
determinator, etc.).

## What this package does NOT do

- Mutate Persons / Employments. The resolver returns intentions; the
  normalize action commits them.
- Decide capability grants. EmployeeRecords don't carry access; that
  comes from `dataset_type=access_grant_record` ingested separately.
