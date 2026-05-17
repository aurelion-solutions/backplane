# core/descriptor

Renders an app cartridge's `descriptor.yaml` recipe into a concrete
account descriptor — a `map[string]any` keyed by field name, ready to
be handed to a connector.

## Inputs

```go
type Inputs struct {
    Principal   any            // bound to .Principal in templates
    Application map[string]any // bound to .Application
    TargetState string         // selects the by_state branch
}
```

Principal is typically an inventory record (employee, NHI, …) but the
renderer treats it as opaque — `text/template` walks dot-paths into
maps and structs uniformly.

Application is typically the parsed `AppManifest.Config` map.

## Pipeline applied to every field

1. **Pick the template.** Either the field's `template` directly, or
   `by_state[TargetState]`. A `by_state` field with no entry for the
   target state is omitted from the result entirely (not set to nil).
2. **Execute the Go `text/template`** against the bindings
   (`.Principal`, `.Application`, `.Descriptor`). Templates are
   strict — referencing an undefined key fails the render rather
   than producing the literal `<no value>`.
3. **Apply post-template transforms** in declaration order.
4. **Resolve `on_collision`** via the registry (or `StubResolver`
   when the name is unregistered or the registry is nil).

## Transforms

Stateless string→string operations applied in pipeline order to the
field's rendered value.

| Name | Effect |
|---|---|
| `lower` | ASCII / Unicode lowercase. |
| `upper` | ASCII / Unicode uppercase. |
| `remove_diacritics` | NFD normalisation + drop combining marks. `Iván` → `Ivan`. |
| `truncate:N` | Clip to N runes (multibyte-safe). |

`lower` and `upper` are also available as in-template helpers via the
FuncMap (`{{ .Principal.Firstname | lower }}`); use whichever reads
better in the YAML.

## Resolvers

Resolvers handle `on_collision` — the rendered+transformed value is
handed to the resolver, which may consult storage and return a
non-colliding replacement (typically a numeric suffix).

```go
type Resolver interface {
    Resolve(ctx context.Context, in ResolverInput) (string, error)
}

type ResolverRegistry map[string]Resolver
```

A field referencing an unregistered name falls back to `StubResolver`,
which returns the input unchanged. This lets the renderer be exercised
end-to-end before the database is wired.

## Cross-field references

A template may reference another descriptor field as
`{{ .Descriptor.<name> }}`. `NewRenderer` extracts every such
reference at compile time, builds the dependency graph, and
topologically sorts the fields so each one renders after the fields
it references.

Failure modes detected at compile time, not at render time:

- Reference to an undeclared field.
- A cycle in the dependency graph.

The sort is deterministic (alphabetical tiebreak) so error messages
and test fixtures stay stable across runs.

## Concurrent use

`NewRenderer` is single-shot. The returned `*Renderer` is safe for
concurrent use — `Render` writes only to a per-call result map and
never mutates the compiled state.

## Raw scalar values

A `by_state` value that is a YAML number / bool (`active: 512`) is
passed through unchanged — no template execution, no transforms, no
collision resolver. The descriptor map ends up with the original type
(`int(512)`, not `"512"`).

## HTTP surface

`RegisterRoutes(g, provider)` mounts one endpoint:

```
POST /cartridges/{id}/apps/{app_id}/descriptor
```

Request body:

```json
{
  "principal":    { "Firstname": "Iván", "Lastname": "Müller", "OrgUnit": "engineering" },
  "application":  { "Domain": "staging.example.com" },
  "target_state": "active"
}
```

`principal` is bound to `.Principal` in templates. `application`
overlays the cartridge manifest's `config:` block — keys in the
request win, keys not in the request fall back to the manifest
default. `target_state` is mandatory and must be one of the states
declared in the app cartridge's `account.yaml`.

Response:

```json
{ "fields": { "userPrincipalName": "ivan.muller@staging.example.com", "ou": "...", "userAccountControl": 512 } }
```

Status codes:

- `200` — render succeeded
- `400` — `target_state` missing or body malformed
- `404` — bundle id or app id unknown
- `422` — template execution, transform, or resolver failed (typically
  a missing principal field under `missingkey=error`)
- `500` — `NewRenderer` rejected the cartridge (cycle, undeclared ref,
  unknown transform)

The endpoint cannot live in `core/cartridges` because `core/descriptor`
already imports `core/cartridges` — placing the bridge here keeps the
dependency direction one-way.

## What this package does NOT do

- No database access. The default resolver is a no-op stub. Real
  collision handlers (database-backed) live in the consumer layer
  (the future `access_apply` engine).
- No state-machine validation. The renderer trusts `TargetState`;
  validating that a transition is allowed is the caller's job.
- No descriptor diffing. Producing the previous + new descriptors
  and computing the change set is the caller's job too.
