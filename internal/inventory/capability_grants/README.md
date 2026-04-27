# capability_grants

Projected (account, capability, scope_key, scope_value) tuples — the
output of the access projector. One row per match between an
AccessGrantRecord and a CapabilityMapping rule.

Natural key (for "who has what" queries):

```
(account_id, capability_id, scope_key_id, scope_value)
```

Lineage key (for idempotency of projection runs):

```
(source_grant_external_id, source_capability_mapping_id)
```

`account_id`, **not** `principal_id` — principals are attached later
by a separate matcher.

## Ingest contract — `dataset_type = access_grant_record`

One record per access assignment fact (`account → resource → action`).

```json
{
  "external_id":     "john.smith::Finance-Admins::member_of",
  "application_id":  "<app-uuid>",
  "account":         "john.smith",
  "resource":        "Finance-Admins",
  "resource_kind":   "ad_group",
  "action_slug":     "member_of"
}
```

- `account` — `account.username` in [`accounts`](../accounts/);
  resolved via `(application_id, username)`.
- `resource` — provider's external_id of the resource; paired with
  `resource_kind` it addresses a row in the lake's `access_artifact`
  contract (read-only, below).
- `external_id` — typically composite, e.g.
  `"{account}::{resource}::{action_slug}"` for per-assignment
  stability.

Projected by [`inventory_normalize.access_grant_record`](../../engines/inventory_normalize/actions/access_grant_record/)
into `capability_grants` through the rules in
[`capability_mappings`](../capability_mappings/).

## Related read-only contract — `dataset_type = access_artifact`

Resource description (group / file / role / authobj). **Lake-only**:
no Postgres mirror — billions of artifact descriptions cost more to
store than they're worth. The projector reads them via DuckDB scan
when a mapping requires `scope_value_source = resource_attribute`.

```json
{
  "external_id":     "S-1-5-21-finance-admins",
  "application_id":  "<app-uuid>",
  "kind":            "ad_group",
  "name":            "Finance-Admins",
  "attrs": {
    "description":    "finance admins",
    "classification": "pii"
  }
}
```

- `kind` + `external_id` — provider-side natural key (resource type
  + source id).
- `attrs` — open-ended; consumed by the projector for
  `scope_value_source = resource_attribute`.
