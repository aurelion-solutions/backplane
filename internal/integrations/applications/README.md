# applications/

Part of Layer-2 `integrations/`. This slice owns the **Application**
domain entity — the catalog of external systems Aurelion governs (an
HRIS, an IdP, a SaaS app, a database). It is the place where "what do we
govern" is recorded.

An Application points at the connector instances that can reach it
through **tag-set matching**: a connector qualifies when its tag set
covers the Application's `required_connector_tags`. The Application owns
the requirement; the live matching lives in [connectors](../connectors/).

## The entity

| Field | Meaning |
|---|---|
| `name` | Human label (1..255 chars). |
| `code` | Stable machine identifier, unique, pattern-validated. The unique key clients pin to. |
| `config` | Free-form JSON for connector/app-specific settings. |
| `required_connector_tags` | The tag set a connector instance must cover to serve this application. |
| `is_active` | Whether the application is in service. Decommission flips it false. |
| `owner` | Accountable party (email / team handle), nullable. Carried as **inventory data**, not UI-managed config — findings on this application's accounts inherit it for routing. |

The snake_case JSON wire shape is locked: `aurelion-engineering-studio`
and other clients pin to these field names, so renaming a field is a
breaking change for every consumer.

## Service rules

- **Create / Update** validate the payload, normalise tags + config to
  non-nil, and translate the Postgres `23505 / uq_applications_code`
  unique violation into the typed `ErrCodeAlreadyExists`.
- **Update** is a partial patch — an empty patch returns `ErrNoFields`.
- **Decommission** flips `is_active` false and emits
  `inventory.application.decommissioned`. It is idempotent: calling it on
  an already-inactive application is allowed and still emits the event.
  Per the engine rules, the **service is the only event emitter** — the
  emit happens after the state change lands, and a failed emit fails the
  call.
- **Delete** hard-removes the row (distinct from decommission, which
  keeps the audit trail).

## HTTP surface

Mounted under `/api/v0`:

| Method | Path | Description |
|---|---|---|
| GET | `/applications` | List all, ordered by name. |
| POST | `/applications` | Create. 409 on duplicate code. |
| PATCH | `/applications/:id` | Partial update. |
| DELETE | `/applications/:id` | Hard delete. |
| GET | `/applications/:id/matching-connector-instances` | Connector instances whose tags satisfy this application (`?online_only=false` to include offline). |

## Cross-slice boundary

The matching endpoint needs the connector registry, but this package
must **not** import `connectors` directly. It declares a narrow
`MatchingProvider` interface (`MatchingForTags(ctx, tags, onlineOnly)`)
and the composition root wires a thin adapter over
`connectors.Service.MatchingForTags`. The return type is intentionally
`any` — the wire shape is owned by `connectors` and serialised
unchanged. Pass `nil` for the matcher only when the connectors slice is
deliberately disabled; the route is then simply not mounted.
