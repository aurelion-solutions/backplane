# workloads

Non-human identity (NHI) kind covering service accounts, machine
identities, and other workload-shaped bodies. First-class slice
rather than a sub-kind of a generic NHI table — future NHI kinds
(API keys, bots, certificates) get their own slice when they ship.

## Shape

`Workload(id, external_id, name, description?, owner_employment_id?,
application_id?, created_at, updated_at)` plus a
`WorkloadAttribute(workload_id, key, value)` EAV sidecar.

- `owner_employment_id` — optional human owner. Used by the principals
  layer to chain status (terminated owner → workload review).
- `application_id` — optional binding to a specific
  [application](../../integrations/applications/).

## No is_locked column — by design

Access blocking for any identity — Employment, Workload, Customer —
lives on the [Principal](../principals/) layer. Workload carries
owner + lifecycle facts only. There is intentionally NO `is_locked`
column here.

A request to "disable workload X" turns into a Principal status
change, not a Workload column flip.

## What this package does NOT do

- Hold credentials. A Workload represents identity; the secrets that
  authenticate it live in
  [`platform/secretmanagers`](../../platform/secretmanagers/).
- Score risk. Risk attaches to the Principal posture, not to the
  body row.
