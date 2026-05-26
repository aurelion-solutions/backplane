# consent

Delegated-access inventory. A consent grant is **evidence of delegated
access, not identity truth**. The application on the receiving end
presents itself in the consent flow, and only the IdP-issued anchor is
trustworthy — its self-asserted name is not.

Two entities:

| Entity | Table | Holds |
|---|---|---|
| `ConsentedApplication` | `consented_application` | The application as it presented itself. Anchor `(source, client_id)`; `display_name` / `publisher` / `home_tenant` / `redirect_uris` are untrusted claims; `verified_publisher` is the one confirmed datum. A resolver may link it to a governed identity (`resolved_principal_id` + `resolution_confidence`); `origin` is derived. |
| `ConsentGrant` | `consent_grant` | The fact that a subject granted an app `scopes`. Natural key `(source, external_id)`. |

## We don't trust the name

In a consent flow an application identifies itself with two very
different kinds of datum, and they must not be conflated:

```
verifiable anchor          self-asserted claims
client_id / app_id         display_name, publisher,
(IdP-issued, trusted)      home_tenant, redirect_uris
                           (the app says so — NOT trusted)
                           verified_publisher = the one
                           datum the IdP actually confirmed
```

So `ConsentedApplication` is keyed on the anchor, and the claims live
beside a `verified_publisher` flag. The posture cartridge flags
unverified publishers, name collisions with governed apps, and apps that
resolve to nothing governed.

## Resolution, not promotion

A resolver may link a presented app to an **already-governed** identity:

```
consent grant → client_id ──resolve──▶ governed principal?
                                        ├─ match → resolved_principal_id set,
                                        │          origin = first_party
                                        └─ none  → unresolved,
                                                   origin = third_party
```

- `resolution_confidence` ∈ `resolved | likely_same | ambiguous |
  unresolved | spoofing_suspected`.
- `origin` ∈ `first_party | third_party | unknown` — **derived** from
  resolution, never asserted by the app.
- An unresolved app is **never** minted into a principal of its own. It
  stays a posture signal. `principals` keeps meaning "an identity we
  govern".

## Scopes live on the grant

The same app receives different scopes from different subjects, so
`scopes` belong on `ConsentGrant`, not on the application. They are
stored **raw** — "high risk" is a policy verdict from the cartridge, not
a stored fact (config in git, not in the row).

- `consenting_principal_id` `NULL` = tenant-wide admin consent or an
  unresolved owner.
- `last_used_at` `NULL` makes a staleness check **not_evaluable** (a
  Blind Spot), not a silent pass.

## API (read-only)

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v0/consented-applications` | List presented apps |
| `GET` | `/api/v0/consented-applications/:id` | One presented app |
| `GET` | `/api/v0/consent-grants` | List consent grants |
| `GET` | `/api/v0/consent-grants/:id` | One consent grant |

App filters: `origin`, `resolution_confidence`, `verified_publisher`,
`resolved`, `resolved_principal_id`. Grant filters: `grant_type`,
`active`, `owned`, `consented_application_id`,
`consenting_principal_id`. Both accept `limit` / `offset`. The write
path (`Upsert`) stays internal to ingest; it is not exposed here.

## Posture

The `ispm-consent-grants` cartridge evaluates presented apps and grants
and emits findings that target the app or the grant directly: unresolved
app with sensitive scope, unverified publisher with high-risk consent,
display-name collision, publisher mismatch, consent without owner, stale
consent, privileged scope to a third party.
