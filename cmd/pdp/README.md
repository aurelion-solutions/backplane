# pdp

**Policy Decision Point** — the request/response process that hosts
AuthZ and AuthN mechanisms. SLO-isolated from backplane / worker:
backplane handles administrative + batch traffic at ~200 ms p99, PDP
needs <10 ms p99 for AuthZ and <5 s for AuthN. Mixing those duty
cycles in one process means a slow batch operation parks the hot path
behind a GC pause.

## What this binary IS

A request/response host for **`engines/policy_assessment` mechanisms
whose budget fits the runtime** — currently `cedar` for AuthZ
decisions, plus future LLM / scoring mechanisms for AuthN flows
(login risk, adaptive MFA, federation trust). The host:

- Boots config + Postgres + RabbitMQ + cartridges provider.
- Composes the engine: `policy_assessment.Store` (in-memory snapshot
  of cartridge-loaded policies, tag-filtered per request) +
  `Dispatcher` registered with the mechanism handlers it supports.
- Boot-loads the store via `Reload(provider)` and prepares every
  registered policy via `dispatcher.PrepareAll`. A watcher polls the
  cartridges provider for changes and re-runs Prepare on the new set.
- Loads snapshot data the handlers need (principals / accounts /
  grants for cedar entity enrichment; signal sources for risk_scoring;
  baselines for behavioral) — TBD; current Cedar handler reads
  `Facts.Entities` supplied by the caller.
- Serves transport endpoints — currently AuthZen 1.0 (`POST
  /access/v1/evaluation`), later SAML / OIDC / Passkey / Federation —
  that translate request envelopes into `policy_assessment.Request`
  (with typed `Facts`), dispatch through the engine, and aggregate
  per-policy `Output`s into the transport's response shape.

### Transport vs internal terminology

AuthZen wire-format calls the actor **`subject`**. The internal
rule contract calls it **`principal`**. The translation happens in
`transport/authzen/handler.go:buildFacts` — `req.Subject` becomes
`Facts.Principal`. Same applies to facets: AuthZen `subject.type`
becomes facet `principal:<type>`. Other transports (SAML / OIDC) do
the same translation in their own `buildFacts`.

## What this binary IS NOT

- **Not the engine.** The engine lives in
  `internal/engines/policy_assessment/`. PDP is just one caller.
- **Not the only caller.** `cmd/worker` calls the same engine for
  scan-budget mechanisms (`sod`, batch `risk_scoring`, etc.).
- **Not a queue consumer for orchestrated work.** Pipeline runs live
  in `cmd/worker`. PDP is request/response only.
- **Not a persistence layer.** PDP touches PG on boot and periodic
  snapshot refresh; the hot path never opens a database connection.

## Current state

**Active.** Cedar handler is wired; AuthZen 1.0 transport
(`POST /access/v1/evaluation`) is live; tag-based pre-filter is in
place; `GET /healthz` reports `policies_total`, `policies_per_mech`,
and `handlers`. SAML / OIDC / Passkey / Federation transports are
not implemented.

## Required env

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `AURELION_PDP_INSTANCE_ID` | yes | — | unique per replica; surfaced in logs |
| `AURELION_PDP_HTTP_ADDR` | no | `:8100` | HTTP bind |
| `AURELION_SECRET_PROVIDER` | no | `file` | secret backend selector |
| `AURELION_SECRETS_FILE` | no | `.secrets.json` | file-backend path |

## Why a separate process

- **SLO isolation.** Backplane GC pauses don't pollute the AuthZ p99.
- **Independent scaling.** Hot-path traffic scales with application
  request volume; backplane scales with admin and integration load.
  Different cost curves, different replica counts.
- **Failure isolation.** A misbehaving mechanism handler that pegs
  CPU on PDP does not take down inventory ingestion or the
  orchestrator.

## Why not also splitting beat into its own cmd

`beat` is already multi-replica-safe inside `backplane` —
`pg_try_advisory_lock` per tick, plus session-level
`pg_advisory_lock` for the matcher. N backplane replicas already
work behind a load balancer. Splitting beat would solve a non-problem
and add a failure mode (a separate process whose absence silently
stalls scheduled work).
