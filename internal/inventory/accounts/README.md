# accounts

Provider user-mailbox inventory. Natural key
`(application_id, username)`. `external_id` and `source` are recorded
for traceability back to the connector batch that produced the row.

Accounts are pure inventory. **Who** stands behind an Account
(employee / NHI / customer) is set by a separate account → principal
matcher that runs AFTER normalize. **What** an Account can do lives
in [`capability_grants`](../capability_grants/).

## Ingest contract — `dataset_type = account`

One record per provider user-mailbox.

```json
{
  "external_id":     "S-1-5-21-1001",
  "application_id":  "<app-uuid>",
  "username":        "john.smith",
  "display_name":    "John Smith",
  "email":           "john.smith@corp.com",
  "is_active":       true,
  "is_privileged":   false,
  "mfa_enabled":     true,
  "status":          "active",
  "attrs":           { "...": "provider-specific" }
}
```

- `application_id` + `username` — natural key in Postgres `accounts`.
  Both required.
- `external_id` — provider's own identifier (AD SID, Okta user id,
  SAP BNAME). Recorded for traceability, not used for matching.
- Other fields denormalize into typed columns; `attrs` is open-ended
  JSONB for the rest.

Normalized by [`inventory_normalize.account`](../../engines/inventory_normalize/actions/account/).
