# connectors/

Part of Layer-2 `integrations/` — the systems Aurelion governs and the
machinery that reaches them. This slice owns the **connector instance
registry** and the **transport + selection** primitives engines use to
dispatch commands to those instances.

A connector is an external backend that self-registers with Aurelion
and executes operations on a governed system (discover accounts, disable
an account, verify a fact). This package does not implement connectors —
it tracks the live ones and speaks the protocol to call them.

## Three concerns

| Concern | Files | What it does |
|---|---|---|
| **Registry** | `model.go`, `repository.go`, `registration*.go` | `ConnectorInstance` rows in Postgres, kept current by the self-registration consumer reading MQ heartbeats. |
| **Selection** | `selector.go` | Pure tag-set matching against the live registry — `required_tags ⊆ instance.tags`, online-only by default. |
| **Transport** | `rpc_client.go`, `descriptor*.go` | A connector-specific wrapper over `core/rabbitmq.RPCClient` that speaks the command/reply protocol. |

## Registry & self-registration

Instances register themselves — Aurelion never provisions them. A
connector publishes `connector.registered` / `connector.heartbeat`
messages onto a topic exchange; `RunRegistrationConsumer` (started in
the composition root) reads them and calls `Service.RegisterFromMessage`,
which upserts the row and stamps `last_seen_at`.

- **Online** = seen within the last 2 minutes (`onlineThreshold`). Pure
  function of `last_seen_at` and the wall clock — no separate liveness
  ping.
- **Stale** = silent for over 24 hours (`StaleCutoff`). Stale rows are
  garbage-collected opportunistically on every register / select pass,
  so the live pool stays honest without a dedicated sweeper.
- Malformed or invalid registration messages are logged and **dropped,
  not requeued** — a poison pill cannot stall the queue.

On registration a connector advertises a `CapabilityDescriptor`: the
operations it supports, the fact kinds it can verify, the legal
account-status transitions it accepts, and the cascade rules
(fact kinds to revoke before `account_disable`). Engines on both sides
of the wire read the same JSON shape.

## Selection

`SelectForTags` picks the first online instance whose tag set covers the
required tags, or `ErrNoMatchingInstance`. `MatchingForTags` returns
*all* matches in the wire shape — this is the `applications.MatchingProvider`
contract, satisfied without forcing `applications` to import this package
directly (the composition root wires a thin adapter).

## Transport (RPC)

`RPCClient` turns a high-level `InvokeRequest` (instance, operation,
payload) into an AMQP request/reply round-trip and unwraps the result:

- **Correlation ID** resolves explicit field → `correlation.ID(ctx)` →
  transport-generated UUID v4, mirroring the kernel contract.
- **Result delivery** is either inline (`payload`) or out-of-band: when
  the reply carries a `result_storage_ref`, the client reads the batch
  from the data lake via an injected `LakeReader` (so a 50K-row discover
  never travels through the message body).
- **Errors**: a non-`ok` status becomes `*ErrRPCStatus` carrying the
  remote status + message; transport timeouts and decode failures wrap
  through verbatim.
- An optional `TraceContext` pins one call into a larger event chain —
  the connector echoes the trace fields back on its result event so
  causation survives the round-trip.

## HTTP surface (read-only)

Mounted under `/api/v0`:

| Method | Path | Description |
|---|---|---|
| GET | `/connector-instances` | List every registered instance. |
| GET | `/connector-instances/:instance_id` | One instance by external id, or 404. |

The registry is written only by the MQ self-registration path — there is
no create/update/delete REST surface. `InstanceWire` is exported because
the `applications` matching endpoint serialises the same shape.

## Boundaries

- The slice does **not** implement connector logic, write to the lake,
  or normalise payloads — it dispatches and tracks. Records flow from
  the connector straight into `inventory_ingest`.
- `Transport` and `LakeReader` are narrow interfaces so tests substitute
  fakes without an AMQP channel or real storage.
- The RPC client lives alongside the service but does **not** depend on
  it — registry use cases and transport are independent.
