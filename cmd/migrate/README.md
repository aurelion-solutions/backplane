# migrate

One-shot Bun migration runner. Applies the migration registry from
`internal/migrations` against the Postgres pointed at by the
configured secret manager.

## Commands

```
migrate init      # create the bun_migrations table (idempotent)
migrate up        # apply every unapplied migration
migrate down      # revert the most recent applied migration
migrate status    # print applied / pending sets
```

`init` is idempotent — calling it on a DB that already has the
table is a no-op. `up` is safe to re-run; it iterates the registry
and only applies what is missing.

## What this binary IS

- The only writer to schema. No backplane / worker / pdp process
  applies migrations on boot.
- A no-arg shell entry. Migrations themselves live in
  `internal/migrations` as `go` files that register against the
  Bun migration registry; this command just drives them.

## What this binary IS NOT

- A long-running service. It runs one command and exits.
- A schema generator. Migrations are hand-written `.go` files in
  `internal/migrations`.
- A data backfill tool. Use a dedicated action / script for that.

## Required env

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `AURELION_SECRET_PROVIDER` | no | `file` | Secret backend selector. |
| `AURELION_SECRETS_FILE` | no | `.secrets.json` | File-backend path. |

The PG DSN is read from the secret manager, same as the other
binaries — no separate flag, no separate env var.

## Operational notes

- `down` reverts **one** migration. To rewind further, run it
  repeatedly. The `down` direction is provided for migrations that
  declare one; many of ours do not, and `down` on those returns an
  error rather than silently doing nothing.
- Concurrent `up` from two operators is safe: Bun's migration
  table takes a row-level lock per migration name.
