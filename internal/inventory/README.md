# inventory/

Domain entities — every persistent identity, access, capability,
policy, and lifecycle row the backplane owns. One slice per
entity. No business workflows live here; capabilities under
`internal/engines/*` and pipelines under `cartridges/` drive change,
inventory carries the state.

## Slices by purpose

**Identity bodies** — the three things a Principal can BE:

| Slice | One-liner |
|---|---|
| [`persons`](persons/) | Canonical human, aggregating Employments via SSN. |
| [`employments`](employments/) | One work period of one Person at one org_unit; the IGA "principal body" — not Person itself. |
| [`workloads`](workloads/) | Non-human identity (service accounts, machine identities); first-class kind, not a sub-row. |
| [`customers`](customers/) | End-user principal independent of Employees/Workloads — the "identity that buys things". |

**Polymorphic identity decision row**:

| Slice | One-liner |
|---|---|
| [`principals`](principals/) | The single point where access posture is decided. One row per body; status derived per-kind. |

**Raw identity ingest (HRIS / AD / SAP side)**:

| Slice | One-liner |
|---|---|
| [`employee_records`](employee_records/) | Raw source-side row about one person from one application. Owns the resolver. |
| [`employee_provider_mappings`](employee_provider_mappings/) | Per-provider rules driving the determinator + upstream-attach resolution. |
| [`employment_record_matches`](employment_record_matches/) | Lineage row: raw record → resolved Employment, one per employment period. |

**Org**:

| Slice | One-liner |
|---|---|
| [`org_units`](org_units/) | Org-tree node; Employments hang off it. |

**Access surface**:

| Slice | One-liner |
|---|---|
| [`accounts`](accounts/) | Application-side identity row (login on a system). Attaches to a Principal. |

**Capability projection** — raw access → business semantics:

| Slice | One-liner |
|---|---|
| [`capabilities`](capabilities/) | Catalog of business actions (`manage_finance_data`, …). |
| [`capability_scope_keys`](capability_scope_keys/) | Catalog of scope dimensions (`department`, `environment`, …). |
| [`capability_mappings`](capability_mappings/) | Admin-written rules: raw access fact → CapabilityGrant. |
| [`capability_grants`](capability_grants/) | Projected output: `(principal, capability, scope_key, scope_value)`. |

**Policy + assessment + workflow definitions**:

| Slice | One-liner |
|---|---|
| [`policies`](policies/) | Policy catalogue row (cartridge-defined; PG mirror of the YAML). |
| [`policy_assessment_runs`](policy_assessment_runs/) | One row per assessment pass — lifecycle, scope, counters. |
| [`findings`](findings/) | Detected violations / anomalies; idempotent via `evidence_hash`. |
| [`pipelines`](pipelines/) | Pipeline catalogue row (cartridge-defined; PG mirror of the YAML). |
| [`initiatives`](initiatives/) | Time-windowed cases / scheduled lifecycle actions. |

**Vocab**:

| Slice | One-liner |
|---|---|
| [`shared`](shared/) | Cross-slice enums + event routing-key catalog (prevents import cycles). |

## Slice skeleton

Every slice that has a service follows the four-file shape; pure
reference / lineage slices stop at `model + repository (+ doc)`.

```
model.go         Bun ORM types + struct tags + CHECK enums
repository.go    PG access, no business rules
schemas.go       Pydantic-equivalent: API I/O shapes (optional)
service.go       Business rules + event emission (only place that emits)
routes.go        Echo handlers; thin — validate → service → JSON (optional)
errors.go        Typed errors (optional)
doc.go           Package-level docstring (when the model alone doesn't read enough)
```

## Rules

- **Naming**: singular class names (`Employee`, `Account`), plural
  route paths (`/employees`, `/accounts`).
- **Events**: emitted only from `service.go`. Models never emit.
- **Status fields** that depend on cross-slice state are owned by
  the slice that computes them — body slices signal, `principals`
  decides.
- **Polymorphism**: discriminated via `kind` plus exactly one
  non-NULL `*_id`. Enforced at the DB layer with a CHECK constraint.
- **Cross-slice vocab** lives in [`shared`](shared/). No slice
  re-exports its sibling's types.

## What this layer does NOT do

- Run policies. That's `engines/policy_assessment`.
- Project access. That's `engines/inventory_normalize/access_grant_record`.
- Schedule. That's `core/orchestrator/beat` + cartridge pipelines.
- Define ingest. That's `engines/inventory_ingest` + the connectors.
