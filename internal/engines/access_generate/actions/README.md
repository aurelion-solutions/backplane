# access_generate.actions

Orchestrator entry points into the access-generate engine. One
sub-package per action.

| Action | Pair | Purpose |
|---|---|---|
| `run` | `("access_generate", "run")` | Recompute the desired initiative set for a single principal and persist the diff. |
