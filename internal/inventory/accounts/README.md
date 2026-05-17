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

## State columns

Every row carries three independent state observations, each owned
by exactly one writer:

| Column | Value vocabulary | Writer |
|---|---|---|
| `effective_state` | `not_exist / pending / blocked / invited / active` | `inventory_normalize.account` (from connector data); `access_apply` (sets `pending` when a command is shipped, until ingest confirms) |
| `desired_state`   | same | `policy_assessment.generative` |
| `validated_state` | same | PDP validator (filters desired through deny grants, SoD, segregation rules) |

`access_apply` reacts to `validated_state ≠ effective_state` — that's
the working set it ships to connectors. The composite index
`ix_accounts_state_divergence (application_id, validated_state,
effective_state)` backs that scan.

Each column has a single-writer setter on the repository:
`SetDesiredState`, `SetValidatedState`, `SetEffectiveState`. `Upsert`
writes `effective_state` on conflict (the connector tells us what
*is*); it leaves `desired_state` and `validated_state` alone so the
observers stay authoritative for their columns.

`pending` means different things on each axis:

- `desired_state = pending`   → generative has not assessed the row yet
- `validated_state = pending` → PDP has not validated it yet
- `effective_state = pending` → a command is in flight; ingest has not
  confirmed the new actual state

Canonical state constants live in `model.go` as `StateNotExist`,
`StatePending`, `StateBlocked`, `StateInvited`, `StateActive`.
