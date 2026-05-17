# core/cartridges

Source-agnostic provider for the cartridge bundle that lives outside
the backplane git repo. Backplane and worker read pipeline YAML and
policy manifests from cartridges; this package is the single boundary
that abstracts where those files come from.

A cartridge is just a directory of static files:

```
<cartridges-root>/<id>/
    pipelines/*.yaml                  ← orchestrator definitions
    policies/<bucket>/*.meta.json     ← policy manifests
    policies/<bucket>/*.<ext>         ← mechanism-specific sibling files
                                        (.cedar, .prompt, .yaml, …)
    apps/<app_id>/manifest.yaml       ← per-application cartridge:
    apps/<app_id>/account.yaml          identity, state machine,
    apps/<app_id>/descriptor.yaml       descriptor recipe
```

This package knows nothing about pipeline grammar or policy semantics —
it only enumerates cartridge ids, materialises files on disk, parses
the app cartridge YAML into typed structs, and reports when something
changes. Higher layers (`orchestrator/loader`,
`engines/policy_assessment`, `core/policies`, the future access_apply)
consume the contents.

## What lives here

| File | Role |
|---|---|
| `interface.go` | `Ref`, `Provider`, `ErrNotFound`, `ErrInvalidManifest`. |
| `manifest.go` | `Manifest` (mirrors the `.meta.json` sidecar) + sidecar loader. |
| `apps.go` | `AppCartridge` (manifest/account/descriptor projection), YAML loader, cross-file validator, `ErrInvalidApp`. |
| `filesystem.go` | `FilesystemProvider` — top-level subdir = cartridge id, recursive walk under `policies/`, flat scan under `pipelines/`, per-app YAML triple under `apps/`. |
| `factory.go` | Named-provider registry. Only `filesystem` is registered today; a future `git` / `oci` / `zip` provider drops in alongside without touching callers. |
| `watcher.go` | `Watcher` — mtime polling helper. Suffix filter, seed-without-trigger first scan, `Run(ctx, interval, onChange)` loop. |
| `routes.go` | `RegisterRoutes(g, provider)` — `GET /cartridges`, `GET /cartridges/:id`, `GET /cartridges/:id/policies`, `GET /cartridges/:id/pipelines`, `GET /cartridges/:id/apps`, `GET /cartridges/:id/apps/:app_id`. Read-only. The render endpoint (`POST /cartridges/:id/apps/:app_id/descriptor`) lives in `core/descriptor` to avoid an import cycle. |

## Provider contract

```go
type Provider interface {
    List() ([]Ref, error)
    Materialize(ref Ref) (string, error)                  // local dir on disk
    Policies(ref Ref) (map[string]Manifest, error)        // rule_id → manifest
    Pipelines(ref Ref) ([]string, error)                  // absolute YAML paths
    Apps(ref Ref) (map[string]AppCartridge, error)        // app_id → parsed app cartridge
}
```

- `Materialize` always returns a local directory path. Providers that
  fetch from elsewhere (git, OCI, zip) extract into a cached
  directory and return its root, so every downstream consumer can
  open files on disk uniformly.
- `Policies` fills `Manifest.BasePath` with the absolute path of the
  `.meta.json` file itself. Mechanism handlers use it as anchor to
  resolve their own sibling files (e.g. `<rule>.cedar`,
  `<rule>.prompt`). The platform does not check sibling file
  existence — that's the handler's job.
- Manifest mechanism is a free-form string. The platform does not know
  domain enums; consumer engines validate it against their own
  allowlist (PDP wants `"generic"`, scan engine adds `"sod"` /
  `"risk_scoring"` / `"llm_classification"` / …).

## Watcher

`NewWatcher(root, opts...)` returns an mtime-poll helper. The first
`Changed()` seeds the state and returns `false` (so consumers do their
boot-time load separately); every subsequent call returns `true` when
any tracked file's mtime, presence, or identity differs from the
previous scan.

`Run(ctx, interval, onChange)` loops at `interval` (default 5 s),
seeds on the first tick, and invokes `onChange` on every diff.
Failures inside `onChange` are logged — the previous in-memory state
stays in effect.

Suffix filter narrows what counts as a "change" for a given consumer:

- `cmd/backplane` + `cmd/worker` watch `.yaml` → rebuild pipeline catalog.
- Mechanism-host processes (Cedar PDP / policy-assessment action in the worker) watch
  `.meta.json` + their mechanism-specific siblings → rebuild handler
  state.
- `core/policies` + `core/pipelines` sync loops do NOT use the watcher —
  they tick on their own 5 s schedule under a Postgres advisory lock.

## What this package does NOT do

- **No grammar / policy-DSL parsing.** That's the consumer's job
  (orchestrator loader for YAML pipelines, mechanism handlers for
  policy bodies).
- **No catalog management.** No in-memory list of every loaded rule —
  consumers keep that.
- **No write path.** Edits land via filesystem changes; no provider
  method writes anything. REST surface is GET-only.
- **No event emission.** Catalog churn does not produce MQ events —
  per-process mtime polling is the design baseline.
