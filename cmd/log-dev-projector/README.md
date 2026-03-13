# log-dev-projector

In-memory log viewer for **local development**. Reads the platform logs
exchange and exposes the most recent events over HTTP so you can `curl`
them instead of standing up a SIEM.

## What it does

1. Connects to RabbitMQ.
2. Declares a durable queue `aurelion.logs.buffer` bound to the
   `aurelion.logs` topic exchange with binding key `#` (catch-all).
3. Consumes every published log Event, parses it as JSON, and pushes
   it into an in-memory ring of size `bufferSize` (default 1000).
   On overflow the oldest event is evicted FIFO.
4. Serves HTTP on `:8001`:
   - `GET /healthz` — liveness probe.
   - `GET /buffer?limit=N` — JSON array of the most recent N events
     (default 100).

## What it does NOT do

- **No database.** Postgres is never opened by this process.
- **No persistence.** Restart the binary, the ring is empty.
- **Not for production.** Use a real SIEM via `log-siem-transmitter`
  for anything that needs durability or compliance.

## Why a separate binary

Same reason as `log-siem-transmitter`: the fan-out topology gives every
queue an independent copy of every log event, so dev tooling and SIEM
delivery can run, fail, and scale independently.
