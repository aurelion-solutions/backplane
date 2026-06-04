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

## Protocols, not brands

A backend is sorted by its **wire protocol**, not its brand. One
protocol is one client; a brand (qwen-local, deepseek, claude…) is a
named config entry pointing at a protocol plus its endpoint. The
OpenAI-compatible client alone covers the local llama-server, OpenAI,
DeepSeek and Mistral — they differ only by `base_url` / `api_key` /
`model`.

| Protocol | Client | File | Speaks for | Status |
|---|---|---|---|---|
| `openai` | `OpenAICompat` | `openai.go` | llama-server (Qwen), OpenAI, DeepSeek, Mistral… | implemented (HTTP+SSE) |
| `anthropic` | `Anthropic` | `anthropic.go` | Claude | implemented (HTTP+SSE) |
| `gemini` | `Gemini` | `gemini.go` | Google Gemini | implemented (HTTP+SSE) |

All three clients stream over HTTP+SSE — one terminal `Chunk` with the
assembled output, even on context cancellation. `OpenAICompat` hits
`/chat/completions`; `Anthropic` hits `/v1/messages` (system is a
top-level field, not a message); `Gemini` hits
`…/models/{model}:streamGenerateContent` (roles `user`/`model`, a
`systemInstruction`). `Stub` remains the embed-able no-op for a future
not-yet-written protocol — `Stream` returns `ErrNotImplemented`; the
`Config` is already threaded through every constructor.

## Config

Named backends live in `config.LLM` (secret key `llm`): a `provider`
naming the active entry, and a `providers` map of name →
`{protocol, base_url, api_key, model}`. The factory is keyed by
protocol; the chosen name resolves to a protocol + `Config`.

## Factory

```go
lf := llm.NewFactory()
llm.RegisterOpenAI(lf)
llm.RegisterAnthropic(lf)
llm.RegisterGemini(lf)

active, err := settings.LLM.Active()        // resolve the named entry
provider, err := lf.Get(active.Protocol, llm.Config{
    BaseURL: active.BaseURL,
    APIKey:  active.APIKey,
    Model:   active.Model,
})
```

Safe for concurrent use. Keyed by protocol — one client per wire
format, reused across brands via `Config`. Model selection lives in
`Config.Model` (and may be overridden per call in `params`).

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
