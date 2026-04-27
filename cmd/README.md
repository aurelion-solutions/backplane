# cmd/

Every backplane process lives here as its own `main.go`. All of them
boot the same way: read `AURELION_SECRET_PROVIDER` / `AURELION_SECRETS_FILE`,
load config via the secret manager, wire postgres / rabbitmq / lake,
then start their specific loop.

| Binary | What it does |
|---|---|
| [`backplane`](backplane/) | HTTP API on `:8000`. Composition root for inventory + integrations + orchestrator coordination (beat, matcher, registration consumer, discover subscriber). |
| [`worker`](worker/) | Orchestrator runner node. Claims pending pipeline runs (`FOR UPDATE SKIP LOCKED`) and executes their steps via the action registry. Scale-out via `AURELION_WORKER_SLOTS` (default 4). |
| [`ingester`](ingester/) | Lake-stream worker. Consumes `aurelion.ingest`, windows records per `(source, dataset_type, correlation_id)`, runs DuckDB anti-join, writes only changed rows to the lake. Requires `AURELION_INGESTER_INSTANCE_ID`. |
| [`log-siem-transmitter`](log-siem-transmitter/) | Bridges the `aurelion.logs` MQ exchange to configured SIEM sinks. |
| [`log-dev-projector`](log-dev-projector/) | Dev-only in-memory log viewer with a small HTTP UI; consumes `aurelion.logs`. |
| [`migrate`](migrate/) | One-shot bun migration runner. `init` / `up` / `down` / `status`. Uses the same secret store as the other binaries. |

Build & run everything locally:

```bash
make build      # produces bin/<name> per row above
make run-all    # foreground multiplex — Ctrl+C kills the whole group
```
