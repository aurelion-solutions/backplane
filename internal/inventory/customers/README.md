# customers

End-user principal independent of Employees and Workloads — the
"identity that buys things" side of IGA.

Each Customer carries tenant scope (optional `tenant_id`,
`tenant_role`), a billing plan tier, MFA-enabled flag, and an
email-verified flag. A Customer is one of the three `PrincipalKind`
bodies (alongside Employment and Workload); the
[`principals`](../principals/) slice owns the polymorphic Principal
row that points at it.

## Shape

`Customer(id, external_id, email_verified, tenant_id?, tenant_role?,
plan_tier?, mfa_enabled, ...)`.

Enum vocabularies (`tenant_role`, `plan_tier`) live in
[`shared`](../shared/) so they don't fork between this slice and the
others that consume them.

## Lifecycle signals

When a Customer's security-relevant posture shifts
(`email_verified` flips, MFA disabled, tenant role changes), the
service emits an event that signals the principals slice to
re-derive Principal status. Customer never writes Principal.Status
directly — that's the principals layer's single point of decision.

## What this package does NOT do

- Decide whether the Customer's Principal is `active` /
  `locked` / `terminated`. Status derivation lives in
  [`principals`](../principals/).
- Hold access grants. Those live on the Principal layer
  ([`accounts`](../accounts/) +
  [`capability_grants`](../capability_grants/)), not on the body.
- Cover authentication. AuthN flow is in `cmd/pdp`; Customer holds
  factual state, not session state.
