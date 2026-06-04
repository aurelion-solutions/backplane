# cmd/

One `main.go` per binary. Every process boots the same way: read
`AURELION_SECRET_PROVIDER` / `AURELION_SECRETS_FILE`, load config
via the secret manager, wire infra, then start its loop.

| Binary | Role |
|---|---|
| [`backplane`](backplane/) | HTTP API + composition root for inventory, orchestrator (beat, matcher), and the cartridge sync loops. |
| [`worker`](worker/) | Orchestrator runner — claims pending pipeline runs and executes step actions. |
| [`ingester`](ingester/) | Lake-stream worker — consumes `aurelion.ingest`, anti-joins, writes the lake. |
| [`pdp`](pdp/) | Policy Decision Point — AuthZ / AuthN request/response host. |
| [`inference-gateway`](inference-gateway/) | The single network entry point for LLM inference — callers stream through it; a GPU worker pool slots in behind the same contract later. |
| [`migrate`](migrate/) | One-shot Bun migration runner (`init` / `up` / `down` / `status`). |
| [`log-siem-transmitter`](log-siem-transmitter/) | Bridges `aurelion.logs` to a SIEM sink. |
| [`log-dev-projector`](log-dev-projector/) | Dev-only in-memory log buffer with an HTTP view. |

Per-binary env vars, SLO targets, and scaling notes live in the
binary's own README.

## Build

```bash
make build      # produces bin/<name> per row above
make run-all    # foreground multiplex — Ctrl+C kills the whole group
```
