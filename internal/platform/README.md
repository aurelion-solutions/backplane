# platform/

Pluggable external integrations. Each subdirectory is one capability
that has a contract (`interface.go`), a name → constructor registry
(`factory.go`), and one file per shipped backend.

| Package | Role |
|---|---|
| [`secretmanagers`](secretmanagers/) | Secret store — `Manager` / `Mutator` / `FullManager`. File backend wired; Vault / OpenBao / Akeyless / Conjur are stubs. |
| [`siem`](siem/) | Audit / business-event log sink — `Sink` (write) and `Reader` (read-back). File / MQ / Stdout / Multi wired; ELK / Loki / Splunk / Fluentd / QRadar / Seq / Rsyslog / Nagios / Zabbix are stubs. |
| [`storage`](storage/) | Data-lake batch storage — `Storage` (write / read / delete / anti-join). File wired; S3 / Iceberg are stubs. |
| [`llm`](llm/) | Streaming chat-completion provider — `Provider`. All backends (Anthropic / OpenAI / llama.cpp) are stubs today. |

## Layering rules

- `platform/*` MAY import `core/*` (adapter implements a core-defined
  port — though in practice the contracts live inside each platform
  package itself).
- `platform/*` MUST NOT import `core/config` — adapters get raw
  inputs from the composition root, not from `Settings`.
- `core/*` MAY import `platform/*` only via a narrow contract
  (today `core/config → platform/secretmanagers.Manager`).
- Engines / integrations / cmd compose the providers; the platform
  packages themselves never know which backend the operator picked.

## Shared shape

Every platform package follows the same skeleton:

```
interface.go     Manager / Sink / Storage / Provider contract + errors
factory.go       Factory (name → Constructor), thread-safe
<backend>.go     one file per shipped backend
stub.go          embeddable Stub{} → every method returns ErrNotImplemented
```

A provider not yet wired embeds `Stub{}` and exposes a `Register*`
helper anyway — the operator can configure the name in `.secrets.json`
and the boot fails fast with `ErrNotImplemented` instead of "unknown
provider".

## Build

These packages have no main; they are imported by:

| Caller | What it pulls |
|---|---|
| `cmd/backplane` | all four |
| `cmd/worker` | `siem`, `secretmanagers` (events go through `core/events`, not here) |
| `cmd/ingester` | `storage`, `secretmanagers` |
| `cmd/pdp` | `secretmanagers` |
| `cmd/migrate` | `secretmanagers` |
