# log-siem-transmitter

Bridges the platform logs exchange to a real SIEM (or file, for dev).
This is the **production** delivery path for log events.

## What it does

1. Connects to RabbitMQ.
2. Declares a durable queue `aurelion.logs.siem` bound to the
   `aurelion.logs` topic exchange with binding key `#` (catch-all).
3. Builds a `siem.Factory` with all known providers (file, mq, elk,
   loki, splunk, qradar, fluentd, zabbix, nagios, rsyslog, seq) and
   picks one by the `siemProvider` constant (default `file`).
4. For each delivery: parses JSON into a `siem.Event` and calls
   `sink.Emit(ctx, event)`. Successful emit → ack. Parse error →
   drop without requeue. Emit error → nack without requeue
   (dead-letter territory).

## What it does NOT do

- **No HTTP, no buffer.** Pure MQ → sink bridge.
- **No retries.** Failed emits go to nack — wire dead-letter exchange
  in RabbitMQ if you need durability beyond the broker.
- **Not for log browsing.** Use `log-dev-projector` for that.

## Why a separate binary

Fan-out: backplane publishes once to the `aurelion.logs` exchange;
both `aurelion.logs.siem` and `aurelion.logs.buffer` queues each
receive a copy. SIEM delivery and dev tooling fail and scale
independently.

## Switching SIEM backend

Edit the `siemProvider` constant in `main.go`. Only `file` and `mq`
are real today; the rest are stubs that return `ErrNotImplemented`
from `Emit` — replace the matching type in
`internal/platform/siem/` to ship a real backend.
