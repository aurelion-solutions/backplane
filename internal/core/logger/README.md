# logger

Process-wide `*slog.Logger` constructor. Pure plumbing — no env
parsing, no domain knowledge, no logfile management.

## Contract

```go
log := logger.New(logger.Config{
    Writer: os.Stdout,
    Level:  slog.LevelInfo,
    Format: logger.FormatJSON,
})
```

The caller decides the writer, the level, and the format. The
package returns a logger that stamps `correlation_id` from context
when a handler is wrapped with `WithCorrelationID`.

## What this package does NOT do

- Read env vars or config files. `cmd/*` composes the `Config`
  from `core/config`.
- Manage log rotation or file handles. Pass any `io.Writer`.
- Replace `slog`. The returned value is a stdlib `*slog.Logger` —
  callers use it directly with `log.Info`, `log.With`, etc.
- Emit domain events. Those go through `core/events`.
