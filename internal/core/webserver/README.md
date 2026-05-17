# webserver

Echo HTTP server with the bootstrap middleware stack and a single
`/healthz`. Pure plumbing — every cmd that exposes HTTP composes
its routes onto the `*echo.Echo` returned here.

## Middleware stack

In order:

1. **Recover** — turns a handler panic into a 500 instead of taking
   the process down.
2. **Request id** — stamps `X-Request-Id` on the request context.
3. **Correlation id** — reads `X-Correlation-ID` from the inbound
   request or mints a fresh UUID, then stores it on the context
   under `core/correlation`'s key. Outbound responses echo it back.
4. **CORS** — origins and methods come from `Config.CORS`. Default
   policy is dev-friendly (`*`); production callers narrow it.
5. **slog access log** — one structured line per request with
   method, path, status, latency, request id, correlation id.

## Endpoints

- `GET /healthz` — `200 {"status":"ok"}`. Mounted by this package
  so every cmd has it without copying.

## Caller pattern

```go
e := webserver.New(webserver.Config{...})
v0 := e.Group("/api/v0")
inventory.RegisterRoutes(v0, deps)
log.Info("listening", "addr", cfg.Addr)
_ = e.Start(cfg.Addr)
```

The returned `*echo.Echo` is yours — mount any group, register any
handler. The package does not own routing beyond `/healthz`.

## What this package does NOT do

- Authenticate or authorise. Auth is handler-side, not middleware
  here.
- TLS termination. The reverse proxy or `e.StartTLS` handles that.
- Rate limiting. Add it at the gateway tier.
