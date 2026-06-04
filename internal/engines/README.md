# engines/

Layer 2 of the backplane architecture. Engines own reusable
capabilities that products compose into features. They sit above
`core/` and `platform/` (infrastructure) and below the product layer.

## What lives here

An engine is a self-contained slice that owns a single capability
end-to-end: its models, persistence, service logic, REST surface, and
the events it emits. Engines are reusable — the same engine is
consumed by IGA, IDP, future PAM, etc. They are not product-specific.

**Inventory pipeline** — raw records into governed domain state:

| Engine | Role |
|---|---|
| [inventory_discover](inventory_discover/) | Orchestrates a pull: tells a connector to start discovery and tracks the run until it completes. Does not touch the lake itself. |
| [inventory_ingest](inventory_ingest/) | The single writer into the data lake. Hashes each record, dedupes against the latest revision per external_id, writes only what changed, emits `inventory.ingest.batch_received`. |
| [inventory_normalize](inventory_normalize/) | Not a standalone engine — a package of orchestrator actions the worker runs to turn a raw lake batch into domain entities (accounts, employments, …). |
| [inventory_import](inventory_import/) | Synchronous CSV-import façade: runs ingest + normalize for one dataset in a single request inside one transaction. |

**Access** — what a principal should hold:

| Engine | Role |
|---|---|
| [access_generate](access_generate/) | Computes the initiatives a principal *should* hold by projecting structural state (employment, OU), ITSM requests, and delegations through cartridge rules; writes `desired_state`. |

**Assessment** — turning state into findings and narratives:

| Engine | Role |
|---|---|
| [policy_assessment](policy_assessment/) | The single dispatcher every caller goes through to evaluate one or many policies; N mechanism handlers (cedar, sod, llm_classification, …) each own one class of evaluation. |
| [risk](risk/) | Deterministic 0..100 priority scoring, factor-decomposed so every score is explainable without any model or AI. |
| [owner_assignment](owner_assignment/) | Resolves a finding's accountable owner from its account's application (ownership carried as inventory data, resolved in-memory per run). |
| [compliance_projection](compliance_projection/) | Read-time view that rolls an assessment run's existing findings up onto external compliance control languages (SOC 2 logical access, …); persists nothing. |
| [finding_explanation](finding_explanation/) | Turns an already-proven finding into a cited, human-readable narrative via the inference gateway. Explains findings; never creates them. |

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
