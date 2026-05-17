# postgres

`*bun.DB` constructor bound to a pgdriver connection pool. Pure
infrastructure — every backplane component that touches PG goes
through here, and there is exactly one instance per process.

## Contract

```go
db, err := postgres.New(postgres.Config{
    DSN:             dsn,
    MaxOpenConns:    50,
    MaxIdleConns:    10,
    ConnMaxLifetime: 30 * time.Minute,
})
```

The caller composes `Config` from `core/config` and passes it in.
The package opens the pool, pings once, and returns the `*bun.DB`.
Process shutdown closes the pool via `db.Close()`.

## Conventions

- One `*bun.DB` per process. Repositories accept `bun.IDB` so they
  work both with the pool and with a transaction handle.
- Transactions are owned by the call site (`db.RunInTx`), not by
  repositories. Repositories never start a transaction of their
  own.
- The package emits no metrics and no events. Logging is the
  pool's responsibility (configured on `Config.Logger`).

## What this package does NOT do

- Migrations. That's `cmd/migrate` plus `internal/migrations`.
- Schema definitions. Those live in the inventory slice that owns
  the table.
- Retry or reconnect logic. The pool handles transient errors;
  bigger outages surface to the caller.
