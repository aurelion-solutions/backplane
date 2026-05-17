# principals

Polymorphic abstraction over the three identity bodies:
[Employment](../employments/), [Workload](../workloads/),
[Customer](../customers/). The single point in the inventory layer
that makes access decisions.

Each Principal row carries a `kind` discriminator plus exactly one
`principal_*_id` foreign key pointing at its underlying body.

## Shape

`Principal(id, kind, principal_employment_id?, principal_workload_id?,
principal_customer_id?, status, locked_reason?,
status_recomputed_at, ...)`.

Exactly one `principal_*_id` is non-NULL — enforced by a DB CHECK
constraint matching `kind`.

`kind` vocabulary lives in [`shared`](../shared/) as
`PrincipalKind`. Status enum is kind-specific (see
`status_derivation.go`).

## Status derivation

Body slices (employments, workloads, customers) **do not** write
`Principal.Status` directly. They emit a "body changed" signal; the
principals service runs the kind-specific derivation and updates
the row.

`status_derivation.go` holds the rules — what makes an Employment
Principal `active` vs `locked` vs `terminated`, etc. Single
location, single decision.

## Access posture

Access grants (Accounts, CapabilityGrants) attach to the Principal,
not to the body. Workload-owned access stays attached even when the
Workload changes owner; Customer access never silently inherits
Employment access because they are different Principals.

## What this package does NOT do

- Decide AuthN. Authentication is `cmd/pdp`; Principal is the AuthZ
  subject, not the AuthN session.
- Derive body state. Bodies own their own fields; the principals
  layer only reads body state to compute derived status.
- Notify. A status change emits a `principal.status_recomputed`
  event; downstream notifications belong to notification engines,
  not here.
