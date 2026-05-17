# capabilities

Catalog of business-meaningful actions — `manage_finance_data`,
`read_prod_data`, `post_financial_documents`, etc. Pure reference
data, admin-managed.

A Capability is identified by `slug` — the stable identifier that
[`capability_mappings`](../capability_mappings/) reference when
projecting raw access facts into business semantics.

## Shape

`Capability(id, slug, name, description?, is_active, created_at, updated_at)`.

No service layer — the catalog is small, read-mostly, and accessed
through the repository directly by capability-mapping projection
code. CRUD lives in the admin surface, not as a domain workflow.

## Relationships

- One Capability → many [`CapabilityMapping`](../capability_mappings/)
  rules that target it.
- One Capability → many [`CapabilityGrant`](../capability_grants/)
  projections (output of the mapping engine).
- One Capability → many policy obligations that reference it by
  slug.

## What this package does NOT do

- Project access facts → CapabilityGrants. That's
  `inventory_normalize.access_grant_record` driven by the mapping
  rules.
- Score / risk-rate capabilities. Risk is a policy-layer concern,
  not catalog metadata.
