# secrets

Authentication-evidence inventory. A secret is **not** an identity — it
is a thing an actor presents to authenticate. The identity behind it is
resolved separately, on the `principal_id` axis.

Two entities, split by shape:

| Entity | Table | Holds |
|---|---|---|
| `SecretPlain` | `secret_plain` | Opaque material: `password`, `connstring`, `token`, `api_key`. Carries token `scopes` and a value `fingerprint`. |
| `SecretCertificate` | `secret_certificate` | PKI material: `x509` / `openssh` certs and keys. Carries `usage[]` and the structured PKI fields (subject, issuer, serial, key algorithm/size, `self_signed`, validity window). |

Natural key on both: `(source, external_id)`.

## A secret is an edge, not a node

A secret links **where it was found** to **what it authenticates to**.
Discovery is messy, so every end is nullable; a storage `CHECK` forbids
a secret with no locus at all.

```
[ found_in_application_id / found_in_location ]   the holder / client
                  │
                presents
                  ▼
[ target_application_id + account_id ]            what it authenticates to / as
                  │
              owned by
                  ▼
[ principal_id ]                                  the identity that owns it
```

- A token is usually found in its own target application; a password /
  connection string / key is found in a **client** application's config.
- `principal_id` is the owner/subject. `NULL` = unresolved linkage — a
  posture signal, never a reason to promote a secret into an identity.
- A system token vs a PAT is not a stored sub-kind: it is read from the
  linked principal's kind (`workload` ⇒ system, `employment` ⇒ PAT).

## Lifecycle facts

`issued_at` / `expires_at` (cert: `not_after`) / `rotated_at` /
`last_used_at`. A `NULL` value means the source never evidenced that
datum, so a check depending on it is `not_evaluable` (a Blind Spot)
rather than a silent pass.

## Posture

Evaluated by the [`ispm-credential-posture`](../../../../cartridges/ispm-credential-posture/)
cartridge during the `policy_assessment.assess` secret pass. Findings
target the secret on the `(target_type, target_id)` axis — `target_type`
is `secret_plain` or `secret_certificate`. Checks cover long-lived
secrets, expiry, owner linkage, owner-terminated, weak/self-signed/
over-long certificates, and stale/unverifiable use.

## Read surface

```
GET /api/v0/secrets/plain[/:id]          ?type= ?privileged= ?linked= ?target_application_id= …
GET /api/v0/secrets/certificates[/:id]   ?format= ?privileged= ?linked= …
```

The write path (`Upsert`) is internal to the connector/ingest actions
and is intentionally not exposed over HTTP.
