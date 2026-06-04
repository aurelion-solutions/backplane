# inference-gateway

The single network entry point for LLM inference. Every caller reaches a
model through this process — never through an in-process provider of its
own.

```
backplane / worker / pdp
  -> inference-gateway          (this process)
    -> internal/platform/llm provider
      -> llama-server or cloud provider
```

Keeping inference behind one process means GPU concerns (slots,
batching, model residency) never leak into the API binaries, and the
backend can be swapped or scaled without touching a single caller.

## HTTP surface

| Method | Path | Description |
|---|---|---|
| `POST` | `/v1/inference/stream` | Server-Sent Events token stream. |
| `GET` | `/healthz` | Instance metadata + active backend. |

### `POST /v1/inference/stream`

Request:

```json
{
  "provider": "qwen-local",
  "messages": [{"role": "user", "content": "explain finding X"}],
  "params": {"temperature": 0.2}
}
```

`provider` names a configured entry in `config.LLM.Providers`; omit it
for the configured default. `messages` / `params` pass straight through
— the gateway defines no prompts of its own.

Response is `text/event-stream`, one JSON object per `data:` line:

```
data: {"token": "Privileged", "done": false}
data: {"token": " account", "done": false}
data: {"output": "Privileged account…", "tokens_used": 42, "done": true}
```

A backend that is not wired yet fails before the stream opens and is
returned as a plain JSON error with `501 Not Implemented` — no SSE is
started.

## Executor — the swap point

Inside, the gateway holds an `Executor`. Today there is one
implementation, `LocalExecutor`: it resolves the requested named backend
to a protocol + `Config`, builds the provider from
`internal/platform/llm`, and streams **in-process**. No worker pool, no
scheduler.

## Planned — worker pool behind the gateway

When GPU slots need scaling, a lower layer slots in **behind the same
HTTP contract** — callers do not change:

```
backplane / worker / pdp
  -> inference-gateway
    -> inference-worker pool      (planned)
      -> llama-server
```

A `DistributedExecutor` replaces (or sits alongside) the local one and
fans requests out to a pool of inference workers. Workers are expected
to advertise **tags** describing what each can serve — model family /
quantization, modality, residency, hardware class — so the gateway can
route a request to a worker that actually holds the right model instead
of treating the pool as uniform. The local executor stays available for
single-host / dev runs; the gateway picks the executor at composition
time.

The point of building the gateway first is exactly this: we get the
process boundary and the stable API now, and add the slot scheduler only
when load demands it — without an architectural rewrite.

## Run

```bash
go run ./cmd/inference-gateway
# or, built:
./bin/inference-gateway
```

Boots like every other binary — `AURELION_SECRET_PROVIDER` /
`AURELION_SECRETS_FILE` → config via the secret manager → wire the llm
factory → serve. It needs no Postgres, RabbitMQ, or cartridges.

| Env | Default | Purpose |
|---|---|---|
| `AURELION_INFERENCE_GATEWAY_HTTP_ADDR` | `:8090` | HTTP bind address. |
| `AURELION_SECRET_PROVIDER` | `file` | Secret backend. |
| `AURELION_SECRETS_FILE` | `.secrets.json` | Secrets path (file provider). |

Backend selection (active provider, per-protocol endpoints, keys) lives
in the `llm` config section — see `internal/platform/llm/README.md`.
