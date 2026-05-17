# loader

Parses pipeline YAML out of cartridge `pipelines/` directories and
hands back validated `Definition` structs. Every catalog reload
goes through here.

## Validation order

Fixed and fail-fast — error messages stay deterministic across runs:

1. **YAML parse** to a generic mapping.
2. **Structural grammar** via `core/orchestrator/grammar` — fails
   the load on schema violations (wrong types, missing required
   keys).
3. **Semantic checks** — step names unique, `requires` references
   resolve, no cycles, template refs (`${args.X}` / `${steps.S.result.Y}`)
   point at known sources.
4. **Build** the in-memory `Definition` ready for the runner.

A bad pipeline drops out at the earliest failed gate; the rest of
the cartridge bundle still loads.

## Layout

| File | Role |
|---|---|
| `loader.go` | Entry point: `Load(provider, ref)` → `[]Definition, []error` |
| `definition.go` | The runtime `Definition` shape |
| `templating.go` | Template-reference detection used during semantic checks |
| `regex.go` | Compiled patterns used by the validators |
| `errors.go` | Typed load errors |
| `doc.go` | Package overview |

## What this package does NOT do

- Execute pipelines — that's `core/orchestrator/runner`.
- Schedule pipelines — that's `core/orchestrator/beat`.
- Mirror to PG — `core/pipelines` does it after the catalog stabilises.
