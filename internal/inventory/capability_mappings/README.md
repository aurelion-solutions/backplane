# capability_mappings

Admin-written rules that translate raw access facts
(AccessGrantRecord) into business-meaningful CapabilityGrants. The
projection catalog — one rule per row, each rule a filter plus a
value source.

## Rule shape

A rule combines:

- **One [Capability](../capabilities/)** — the business action the
  rule emits.
- **One [ScopeKey](../capability_scope_keys/)** — the dimension the
  emitted CapabilityGrant is scoped on.
- **Resource selector** — XOR over `resource_id` / `resource_kind` /
  `resource_path_glob`. Exactly one is non-NULL; enforced by a DB
  CHECK constraint.
- **Optional filters** — `application_id`, `action_slug`.
- **`scope_value_source`** — JSONB discriminated union describing how
  to compute the CapabilityGrant's `scope_value` at projection time.

## scope_value_source kinds

```json
{ "kind": "constant",            "value": "<string>" }
{ "kind": "application_id" }
{ "kind": "principal_attribute", "key":   "<attr key on Account>" }
{ "kind": "resource_external_id" }
{ "kind": "resource_attribute",  "key":   "<attr key on AccessArtifact>" }   // lake lookup; not implemented
```

A NULL evaluation result yields a GLOBAL CapabilityGrant in that
dimension — see [`capability_scope_keys`](../capability_scope_keys/).

## Lifecycle

- Loaded by the projection action in `inventory_normalize.access_grant_record`.
- Re-applied on every access fact change — the engine deletes and
  re-emits CapabilityGrants per affected `(principal, capability,
  scope_key)` triple.
- Admin-managed: there is no service workflow today; CRUD lives in
  the admin surface and goes through the repository directly.

## What this package does NOT do

- Hold the projected output. CapabilityGrants live in
  [`capability_grants`](../capability_grants/).
- Validate selector combinations cross-row. Two overlapping rules are
  allowed — the projection engine just emits both grants.
