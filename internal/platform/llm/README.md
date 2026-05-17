# llm

Streaming chat-completion provider — one contract, pluggable
backends (on-prem `llama.cpp` / GGUF, Anthropic API, OpenAI API,
future providers).

## Contract

```go
type Provider interface {
    Stream(ctx context.Context, messages []Message, params map[string]any) (<-chan Chunk, error)
}
```

A single `Stream` method that returns a receive-only channel. The
channel is **closed** after the provider emits exactly one terminal
`Chunk` with `Done = true` — including on context cancellation.
Cancellation is via `ctx`; there is no separate `Abort`.

## Wire format

```go
type Message struct {
    Role    Role   // "system" | "user" | "assistant"
    Content string
}

type Chunk struct {
    Token      string // streamed fragment; meaningful only while Done=false
    Done       bool
    Output     string // assembled full text (terminal Chunk only)
    TokensUsed int    // (terminal Chunk only)
}
```

Providers MUST:

- emit zero or more non-terminal `Chunk{Token: ..., Done: false}`,
- emit exactly **one** terminal `Chunk{Done: true, Output: ..., TokensUsed: ...}`,
- close the channel after the terminal chunk.

Cancellation rule: the terminal chunk is emitted even when the
context is cancelled — callers can drain the channel without
worrying about a leaked goroutine on the provider side.

## Providers

| Name | File | Status |
|---|---|---|
| `anthropic` | `anthropic.go` | stub |
| `openai` | `openai.go` | stub |
| `llamacpp` | `llamacpp.go` | stub |

Stubs embed `Stub{}` — `Stream` returns `ErrNotImplemented`.

## Factory

```go
lf := llm.NewFactory()
llm.RegisterAnthropic(lf)
llm.RegisterOpenAI(lf)
llm.RegisterLlamaCpp(lf)
provider, err := lf.Get(settings.LLM.Provider)
```

Safe for concurrent use. Coarser-grained than per-model caching —
one provider per backend name; model selection lives in `params`.

## Errors

- `ErrNotImplemented` — stub provider.
- Backend / transport errors propagate as-is.

## What this package does NOT do

- Define prompts. Prompts and `params` are the caller's concern;
  this package is the transport.
- Token-count or cap output. `TokensUsed` is reported, not enforced.
  Quota / rate-limiting lives in whichever capability calls `Stream`.
- Persist the conversation. Sessions / histories belong to the
  caller; the provider is stateless across `Stream` invocations.
