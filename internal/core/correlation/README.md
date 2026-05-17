# correlation

Request-scoped correlation id carried across goroutines on
`context.Context`. One id per inbound request — propagated to every
event, log line, and downstream RPC made on the request's behalf.

## Contract

- The id enters the process from the inbound HTTP request, header
  `X-Correlation-ID`. If the header is missing a fresh UUID is
  minted.
- The id lives on the request context under a private key. Reading
  it on a context that never had one returns the empty string.
- Outbound paths (events.Envelope, MQ publishers, RPC clients) stamp
  the id so consumers downstream can reconstruct the chain.

## API

```go
ctx := correlation.With(parent, id)
id  := correlation.From(ctx)
```

Two entry points only — `With` and `From`. Nothing else.

## What this package does NOT do

- Generate the id outside of HTTP. Other entry points (cron beat,
  MQ consumer, CLI) mint their own UUID and attach it at boundary
  time — `correlation` exposes `With` for them, not a generator.
- Persist the id. It is a per-request transient.
- Log or emit anything. Loggers and the events sink read it; this
  package only stores it.
