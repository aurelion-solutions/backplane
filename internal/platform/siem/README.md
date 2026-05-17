# siem

Audit / business-event log sink. Carries structured records that
describe **what the system did** — "who acted on whom, when, with
which correlation chain" — out to file, MQ, or a SIEM backend.

Not the process logger. `core/logger` is the in-process slog setup;
`siem` is the destination for events that survive the process and
land in an external system of record.

## Contracts

```go
type Sink interface {
    Emit(ctx context.Context, event Event) error
}

type Reader interface {
    Read(ctx context.Context, limit int) ([]Event, error)
}
```

`Sink` is write-only. `Reader` is optional — most external SIEMs are
write-only and don't implement it; the file backend does.

## Event shape

`Event` carries the trace triple — initiator / actor / target — plus
`correlation_id` and an optional `causation_id`. Build via:

- `NewRoot(RootInput)` — trace-root, generates `event_id` and
  `correlation_id`.
- `NewDownstream(parent, DownstreamInput)` — inherits `correlation_id`,
  sets `causation_id` to the parent's `event_id`.
- `NewDownstreamFromParentID(parentEventID, correlationID, ...)` —
  when only the IDs are known (connector RPC, MQ deliveries).

`validate()` rejects empty messages, unknown participant kinds, and
self-referential causation.

## Providers

| Name | File | Status |
|---|---|---|
| `file` | `file.go` | wired — JSON-lines on disk, `FileReader` available for read-back |
| `mq` | `mq.go` | wired — AMQP topic publisher |
| `stdout` | `stdout.go` | wired — dev / container output |
| `multi` | `multi.go` | wired — fan-out to several sinks |
| `elk` | `elk.go` | stub |
| `loki` | `loki.go` | stub |
| `splunk` | `splunk.go` | stub |
| `fluentd` | `fluentd.go` | stub |
| `qradar` | `qradar.go` | stub |
| `seq` | `seq.go` | stub |
| `rsyslog` | `rsyslog.go` | stub |
| `nagios` | `nagios.go` | stub |
| `zabbix` | `zabbix.go` | stub |

Stubs embed `Stub{}` — every method returns `ErrNotImplemented`.

## Factory

`Factory` is a name → `Constructor` registry. The composition root
registers the providers it knows about, then resolves one by name from
settings:

```go
sf := siem.NewFactory()
siem.RegisterFile(sf, "/var/log/aurelion/audit.log")
siem.RegisterMQ(sf, channel, exchange)
siem.RegisterStdout(sf)
// …
sink, err := sf.Get(settings.SIEM.Provider)
```

Safe for concurrent use.

## Levels

`debug` / `info` / `warning` / `error` / `critical`. Anything else is
a validation error at `Event` construction time.

## Helpers

`EmitInfo` is a convenience wrapper for routine "X happened" lines
that don't carry a full participant triple — bootstrap progress,
worker lifecycle messages, etc.

## What this package does NOT do

- Filter / sample events. The caller decides what to emit.
- Buffer / retry. Sinks emit synchronously; durability is the
  backend's concern (MQ → broker, file → fsync semantics).
- Define event taxonomies. `Event` is shape, not schema. Naming
  conventions live in `aurelion-docs`.
