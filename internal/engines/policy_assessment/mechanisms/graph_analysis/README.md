# graph_analysis mechanism

Reasons over relationships between principals, roles, groups,
applications, and initiatives. Used for problems where the structure
of the connection graph is the signal — what Cedar's `in` containment
can't express on its own.

**Output class:** reactive **anomaly** for toxic-path / cycle detection
(`Decision.RiskLevel + Signals`). Can also emit `ProjectedFacts` for
transitive closure expansions (generative path) — TBD.

## When to use

- **Delegation chains** — cycles (`A → B → C → A`) or excessive depth.
- **Group nesting transitive closure** — flatten nested memberships,
  find subjects effectively holding an admin role N hops away.
- **Identity correlation** — link multiple accounts / NHIs likely
  belonging to one physical person (recovery email domain, owner
  phone, shared device).
- **Toxic-path search** — privilege escalation chains across
  applications.

## When NOT to use

- Single-hop containment (`principal in Group::"Admins"`) — use `cedar`.
- Sequence detection over time (use `windowed_threshold`).

## Inputs

- **`Facts.Principal`** under analysis.
- **`Facts.Entities`** — caller may pre-supply the relevant graph slice
  in Cedar entity JSON shape (`uid + attrs + parents`).
- **Graph slice** — when not pre-supplied, the handler loads it from
  inventory tables, SQL recursive CTEs, or a dedicated graph store.

## Algorithm

- Run graph traversal (BFS / DFS / cycle detect / transitive closure)
  using one of:
  - In-process Go graph library for small graphs (<10k nodes).
  - Recursive PG CTE for medium graphs.
  - Dedicated graph store for large (future — Memgraph / Neo4j /
    Janus).
- Emit derived facts (`cycle_present: true`, `closure_size: N`,
  `correlation_cluster_id: ...`) into the assessment output.

## Manifest shape

```jsonc
{
  "rule_id":   "glyph.graph.delegation_cycle",
  "version":   1,
  "name":      "Delegation cycle detection",
  "mechanism": "graph_analysis",
  "severity":  "critical",
  "tags":      ["scan", "graph"],
  "body": {
    "traversal":  {"kind": "cycle_detect", "graph": "delegation", "max_depth": 10},
    "emit":       {"signal_when_cycle": "delegation_cycle_present"}
  }
}
```

## Supporting infrastructure

- Graph loader — pulls subjects / accounts / groups / initiatives /
  delegations into in-memory or graph-store representation.
- Optional caching of materialised closures (TTL based).

## Status

Skeleton. First use case is likely delegation cycle detection — small
enough to ship without a dedicated graph store.
