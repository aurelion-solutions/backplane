# config

Immutable bootstrap configuration. One `Settings` aggregate built at
process start, never mutated afterwards. Every section is read from
a `secretmanagers.Manager` so the same code runs against `.env`,
filesystem secrets, or a future remote secret store with no source
edits.

## Layout

One file per section. Each section has a `Default<X>()` and a
`load<X>(sm secretmanagers.Manager)` helper.

| File | Section | Resolution priority |
|---|---|---|
| `settings.go` | `Settings` aggregate + `Loader` | — |
| `loader.go` | `Load()` + `decodeOptional` helper | — |
| `app.go` | `App` (process-level metadata) | env → secret → default |
| `postgres.go` | `Postgres` (DSN bits) | env → secret → default |
| `rabbitmq.go` | `RabbitMQ` (broker URL + exchanges) | env → secret → default |
| `cartridges.go` | `Cartridges` (provider + root) | `AURELION_CARTRIDGES_ROOT` → secret `cartridges` → default `../cartridges` |

## Contracts

- `Settings` is a plain struct — no methods. Pass it down by value.
- Loaders are **fail-closed**: a malformed secret blob aborts boot.
  Missing optional secrets fall back to defaults.
- Env vars override secret values. Defaults are the lowest tier and
  exist so local dev runs with zero configuration.

## What this package does not own

- The actual secret backend — that's `platform/secretmanagers`.
  `config` only consumes the `Manager` interface.
- Runtime feature toggles — those belong to whichever component
  reads them, not here.
- Tracing / logging level config — `core/logger` consumes
  `Settings.App` and decides for itself.
