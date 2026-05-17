# backplane

The composition root for the main backplane service. Wires every
infrastructure factory (secrets → config → logger → Postgres →
RabbitMQ → events sink → storage → cartridges → inventory services)
together, registers HTTP routes, starts the schedule beat and the MQ
matcher, and serves on the configured address.

## What this binary IS

- The single HTTP surface for **inventory + orchestrator**: every
  `/api/v0/*` route is mounted here.
- The single source of the schedule loop (`core/orchestrator/beat`)
  and the MQ trigger consumer (`core/orchestrator/matcher`).
- The PG mirror sync loops for `policies` and `pipelines` —
  cartridges → DB.
- The owner of the cartridge `Watcher` that feeds catalog reloads
  to itself and to `cmd/worker` (worker re-reads from disk on its
  own; backplane is what keeps the catalog row in PG honest).

## What this binary IS NOT

- A pipeline runner. Step execution lives in `cmd/worker`.
- A lake writer. Ingest goes through `cmd/ingester`.
- An AuthN/AuthZ PDP. That's `cmd/pdp`.

## Wiring order

Fail-fast: an unreachable dependency at start aborts the boot with a
non-zero exit. No process ever boots in a half-ready state.

```
envvars
  → secretmanagers.Factory → secretmanagers.Manager
  → config.Settings
  → logger
  → postgres.DB
  → rabbitmq.Conn (+ exchange declarations)
  → events.Sink
  → storage.Factory
  → cartridges.Provider (+ Watcher)
  → orchestrator.Catalog (loaded from provider)
  → inventory + engine services
  → webserver (echo) + route registration
  → beat loop + matcher consumer + sync loops
  → Serve
```

## Required env

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `AURELION_SECRET_PROVIDER` | no | `file` | secret backend selector |
| `AURELION_SECRETS_FILE` | no | `.secrets.json` | file-backend path |

Everything else (DSN, MQ URL, HTTP bind, cartridge root, …) comes
out of the configured secret manager — see `core/config`.

## Multi-replica

Safe to run N replicas behind a load balancer. The single-leader
loops (`beat`, `policies` sync, `pipelines` sync, matcher
consumer-leader role) are gated by `pg_try_advisory_lock`, so only
one replica per loop ticks at any moment. Failover is implicit —
whichever replica wins the lock on the next tick continues.
