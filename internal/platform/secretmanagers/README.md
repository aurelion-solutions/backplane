# secretmanagers

Secret-store contracts plus every shipped backend, in the same shape as
`platform/siem`, `platform/storage`, `platform/llm` — interface +
factory + one file per backend.

## Interfaces

Three composable interfaces:

```go
type Manager interface {
    Get(name string) (string, error)
}

type Mutator interface {
    Set(name, value string) error
    Delete(name string) error
}

type FullManager interface {
    Manager
    Mutator
}
```

Most callers only need `Manager` — bootstrap configuration reads
secrets at start (`core/config.Load`). Components that actually rotate
or write secrets ask for `FullManager`; trying to write through a
`Manager` is a compile error.

## Providers

| Name | File | Status |
|---|---|---|
| `file` | `file.go` | wired — JSON file, dev-only |
| `vault` | `vault.go` | stub |
| `openbao` | `openbao.go` | stub |
| `akeyless` | `akeyless.go` | stub |
| `conjur` | `conjur.go` | stub |

Stubs embed `Stub{}` — every method returns `ErrNotImplemented`.

## Factory

`Factory` is a name → `Constructor` registry. Providers register
themselves at composition time:

```go
sf := secretmanagers.NewFactory()
secretmanagers.RegisterFile(sf, ".secrets.json")
secretmanagers.RegisterVault(sf)
// …
mgr, err := sf.Get(settings.SecretManager.Provider)
```

Safe for concurrent use.

## Naming

Secret names are flat strings. Convention: `<scope>/<key>` —
`journey/database_url`, `cartridges`, `app/jwt_signing_key`. The
backend may translate that into a file path, a Vault mount, or a
column. Callers never assume.

## Errors

- Missing secret → typed `ErrNotFound`. Callers check this and fall
  back to defaults (in `core/config`) or fail closed (in bootstrap).
- Stub providers → typed `ErrNotImplemented`.
- Backend errors propagate as-is.

## What this package does NOT do

- Validate secret content. A secret is an opaque string. Parsing is
  the caller's job.
- Cache. Callers cache at the layer that knows the lifetime — most
  often `core/config` after `Load()`.
- Rotate. Rotation is a runtime capability on top of `Mutator`, not
  something the provider handles automatically.
