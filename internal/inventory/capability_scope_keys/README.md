# capability_scope_keys

Catalog of scope dimensions — `department`, `company_code`,
`environment`, `data_class` — that narrow a CapabilityGrant. Pure
reference data, admin-managed.

Without a scope key, `manage_finance_data` would be either fully
GLOBAL or unusable. With one, the same capability can be granted
narrowed (`scope_value = "AP-NL"`) per row.

## Shape

`CapabilityScopeKey(id, code, name, description?, is_active, created_at, updated_at)`.

`code` is the stable identifier referenced by
[`capability_mappings`](../capability_mappings/) and
[`capability_grants`](../capability_grants/).

## Semantics of NULL scope_value

A CapabilityGrant whose `scope_value` is NULL means the capability
applies **globally** in that scope dimension — no narrowing. NULL is
a vocabulary choice, not "data missing".

## What this package does NOT do

- Enforce a closed set of allowed scope values per key. Values are
  free-form strings; validation lives at the policy layer where
  needed.
- Track per-capability eligibility. Whether `data_class` is a valid
  dimension for `manage_finance_data` is encoded by the mapping
  rules, not in this catalog.
