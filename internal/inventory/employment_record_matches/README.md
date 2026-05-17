# employment_record_matches

Lineage row linking one raw lake [EmployeeRecord](../employee_records/)
to one [Employment](../employments/) row, for **one employment period**
inside that record.

A source record (HRIS `dataset_type=employee` payload) can list
several employment periods inline. Each period gets its own match
row, discriminated by `period_start_date`.

## Shape

`EmploymentRecordMatch(id, employment_id, source,
source_record_external_id, period_start_date, matched_via_determinator,
...)`.

Uniqueness is on `(source, source_record_external_id,
period_start_date)` — the natural key from the lake.

`matched_via_determinator` distinguishes the authoritative match path
(HRIS-style) from the upstream-attach path (AD-style) — see
[`employee_provider_mappings`](../employee_provider_mappings/).

## Cardinality

- Multiple match rows per Employment are allowed: an HRIS record AND
  an AD record can both attach to the same Employment when both
  describe the same job from different angles.
- Two different Employments can NEVER share `(source,
  source_record_external_id, period_start_date)`.

## What this package does NOT do

- Hold record payload. The raw record lives in the lake; this row
  carries the natural key only.
- Hold attributes. EmployeeRecord attributes are in
  [`employee_records`](../employee_records/); resolution-side
  attributes propagate onto Person / Employment by the normalize
  action.
