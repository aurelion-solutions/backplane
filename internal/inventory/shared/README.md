# shared

Cross-slice vocabularies and constants for the inventory layer.

This package exists to prevent import cycles between sibling slices
that need to reference each other's vocabularies — `principals` needs
`PrincipalKind`, `customers` needs `CustomerPlanTier`, the body
slices need a shared event-routing-key catalog, and so on. Putting
the vocab here keeps the relations one-way: every slice imports
`shared`; nothing in `shared` imports a slice.

## Contents

| File | Holds |
|---|---|
| `enums.go` | `PrincipalKind`, status enums for the IGA-universal body vocabularies, `CustomerPlanTier`, `CustomerTenantRole`, … |
| `events.go` | Routing-key constants for every inventory event — `inventory.person.created`, `inventory.principal.status_recomputed`, etc. |

## Why event constants live here, not per-slice

- Several events are emitted across slice boundaries — e.g.
  `inventory.principal.status_recomputed` originates in the
  principals slice but the trigger fires from `customers`,
  `workloads`, or `employments` changes.
- Keeping the catalog central makes it impossible for producer and
  consumer to silently diverge on the routing-key spelling.

## What this package does NOT do

- Hold logic. No services, no repositories. Constants and small
  validation helpers (`Valid()` methods) only.
- Hold cross-layer vocabularies. Engine / policy / pipeline
  vocabularies live in their respective layers.
- Re-export upstream types. Each constant is defined here, even if
  the same string appears in a kernel migration — the inventory
  layer owns its enum vocabulary directly.
