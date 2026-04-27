# engines/

Layer 2 of the backplane architecture. Engines own reusable
capabilities that products compose into features. They sit above
`core/` and `platform/` (infrastructure) and below the product layer.

## What lives here

An engine is a self-contained slice that owns a single capability
end-to-end: its models, persistence, service logic, REST surface, and
the events it emits. Engines are reusable — the same engine is
consumed by IGA, IDP, future PAM, etc. They are not product-specific.

| Engine | Role |
|---|---|
| [inventory_ingest](inventory_ingest/) | Receives raw records, hashes them, dedupes against current lake state, writes only changed revisions to the lake. |
| [inventory_discover](inventory_discover/) | Orchestrates a pull: tells a connector to start discovery and tracks the run until it completes. |

The inventory chain will continue with `inventory_normalize`,
`inventory_reconcile`, and `inventory_apply` — but those are not
written yet; they will live in this directory when they are.

## Rules every engine follows

- **One responsibility.** An engine does the thing in its name and
  nothing else. If a step grows two concerns, it splits into two
  engines.
- **No cross-engine imports.** Engines that need each other (e.g.
  `inventory_discover` calls `inventory_ingest.Register`) accept a
  narrow port interface and the composition root wires the concrete
  type in. The engine package never imports another engine's
  symbols directly.
- **Service is the only event emitter.** Models and repositories
  never publish events. Routes do not publish events. Only
  `service.go` does, and exactly after the corresponding state
  transition lands.
- **Routes are thin.** A handler parses input, calls one service
  method, maps errors to HTTP codes, returns the response. No
  business logic, no fetch logic, no event emission.
- **No silent writes.** Every persisted state change has a
  corresponding emitted event so downstream engines can react. If an
  emit fails, the call fails — exception: observability-only events
  whose loss would orphan a run row are emitted best-effort with a
  documented comment.
- **No direct lake access except `inventory_ingest`.** The data lake
  has exactly one writer. Engines that source raw data hand it to
  `inventory_ingest.Register` instead of writing themselves; that
  keeps the `inventory.ingest.batch_received` event the single
  source of truth for "a new batch landed."

## File layout

```
<engine_name>/
├── doc.go         # package overview, Go-tooling friendly
├── README.md      # human-readable overview
├── model.go       # Bun ORM rows + enums
├── errors.go      # sentinel errors
├── events.go      # event-type constants
├── repository.go  # persistence boundary + Bun implementation
├── schemas.go     # REST envelopes + validation
├── service.go     # use case layer; only event emitter
└── routes.go      # echo handlers
```

`doc.go` and `README.md` deliberately overlap — `doc.go` is what
shows up in `go doc`, `README.md` is what shows up in directory
listings on GitHub.
