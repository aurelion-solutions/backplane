# normalize.account

Action for `dataset_type = account`. Nothing fancy: flat upsert into
the `accounts` table.

## Why

Account is an **inventory catalogue** of user-mailboxes in
applications. The question "who is assigned what?" is not answered
here — that is [`access_grant_record`](../access_grant_record/). The
question "who stands behind this account in real life?" is also not
here — that is a separate account → principal matcher that runs
AFTER normalize.

Account is needed on its own as a per-application identifier with
basic attributes (display_name, email, is_active, is_privileged,
mfa_enabled, status).

## Natural key

`(application_id, username)`.

If the connector resends the same `username` for the same
`application_id`, it is the **same account** — we UPDATE. If the
username has changed, that is a **new account**, and the old row
stays (older grants may still reference it).

## Algorithm

1. Read `lake/account/*.jsonl` for the given batch_id (via the
   `correlation_id` in meta).
2. For each record:
   - validate required payload fields: `application_id`, `username`
   - UPSERT into `accounts` keyed on `(application_id, username)`
3. Emit `inventory.normalize.account.upserted` (batch aggregate
   with counts).

## Lake record shape

See the [`account` ingest contract](../../../../inventory/accounts/)
for the full payload schema and field semantics. The action expects
the standard envelope:

```json
{
  "external_id": "S-1-5-21-...-1001",
  "meta": { "hash": "...", "committed_at": "...", "correlation_id": "..." },
  "payload": {
    "application_id": "<app-uuid>",
    "username":       "john.smith",
    "...":            "..."
  }
}
```

`application_id` is the connector's responsibility to place in the
payload (configured via the connector's source → application map);
backplane does not infer it on its own.

## What it does NOT do

- attach the account to a principal — that is a separate engine
  after normalize
- match accounts across applications (no cross-app identity
  resolution; that is on the people side via the determinator
  resolver in [`normalize.employee`](../employee/))
- track source-side divergence — last write wins (this is
  **inventory**, not reconciliation)
