# mechanisms

One subdirectory per `mechanism` value. Each owns the full handler for
one class-of-evaluation problem — its manifest body shape, its
backing infrastructure (DB schema / external client / algorithm), and
the translation of its native result into `policy_assessment.Output`
(carrying a `RuleResult` — `Decision` and/or `ProjectedFacts`).

## Ownership rule

Mechanism A **never** imports from mechanism B. If two mechanisms
share infrastructure (graph loader, baseline cache, LLM client), the
shared piece lives in `core/` and both mechanisms depend on it.

## Mechanism index

| Mechanism | Status | Output class | Manifest body shape |
|---|---|---|---|
| `cedar` | wired | reactive gate (`Decision.Effect`) | sibling `.cedar` text policy |
| `opa` | wired | reactive anomaly + generative (`Decision.RiskLevel + Signals` and/or `ProjectedFacts[]`) | sibling `.rego` text policy |
| `sod` | wired | reactive anomaly (`Decision.RiskLevel + Signals`) | inline `conditions[]` describing capability-slug intersections |
| `risk_scoring` | README only | reactive anomaly (`Decision.RiskLevel + Signals`) | weights map + threshold tiers |
| `behavioral` | README only | reactive anomaly | baseline source + anomaly threshold |
| `llm_classification` | README only | reactive anomaly OR gate (depends on prompt) | `prompt_template_file` + `response_schema` |
| `graph_analysis` | README only | reactive anomaly OR generative | `traversal_kind` + graph slice selector |
| `compliance_scorecard` | placeholder | rollup metrics | TBD |
| `quorum` | placeholder | gate | TBD |
| `windowed_threshold` | placeholder | reactive anomaly | TBD |

## When to use which

The decision starts with the **shape of the question**, not the language:

| Question | Mechanism | Why this one |
|---|---|---|
| "Can principal X do action Y on resource Z?" — request/response AuthZ | `cedar` | Type-checked, formally-verifiable permission language. RBAC + ABAC + ReBAC in one syntax. |
| "Find all records where predicate P holds" — anomaly findings, single-record predicates (orphan account, dormant privileged, terminated principal access) | `opa` | Rego naturally describes derivation over facts. Native time arithmetic, null handling, set ops. |
| "Given principal context, what access should exist?" — generative birthright / joiner / leaver / grace | `opa` | `projected_facts := [...]` is the natural Rego shape. Cedar's one-verdict-per-call does not fit. |
| "Does this principal hold a toxic combination of N capabilities?" — SoD combinatorics | `sod` | DB-backed evaluator over rule sets; idempotency via evidence_hash; mitigation tier resolution. Rego could do it, but persisted rules + admin REST make it its own infra. |
| "Aggregate N signals into one risk score and calibrate to a level" | `risk_scoring` | Weighted aggregation + threshold tiers — a numeric pipeline, not a predicate. |
| "Has the principal deviated from their behavioural baseline?" | `behavioral` | Stateful — needs a baseline store + window. Does not fit pure-eval. |
| "Classify a free-form blob with an LLM" | `llm_classification` | External I/O — Cedar / Rego have no LLM provider access. |
| "Traverse a relationship graph: cycles, transitive closure, toxic paths" | `graph_analysis` | Multi-hop traversal — Cedar `in` containment is too narrow; Rego works but usually wants a real graph backend. |
| "More than N events of type X in window W" | `windowed_threshold` | Stateful counter store. |
| "N-of-M approvers signed off" | `quorum` | Stateful approval state. |
| "Rollup of many findings into one framework scorecard" | `compliance_scorecard` | Aggregator over `findings`, not a primary evaluation. |

### Cedar vs OPA — the binary rule

The boundary between `cedar` and `opa` is simple:

- **Cedar — for deciding about an action.** There is a `principal`,
  there is an `action`, there is a `resource`, and a verdict
  `allow/deny` is required. That is its native semantics.
- **OPA — for deriving a fact.** The action is missing or artificial;
  the rule either answers "does a predicate hold over this snapshot"
  (anomaly) or computes derived facts for the connector (generative).

If the question reads naturally as "**can** X do Y on Z?" — that's
Cedar. If it reads as "**find every** X where Y" or "**which** X must
exist for Z" — that's OPA.

When the question needs combinatorics, aggregation, graphs, time
windows, LLMs, or real-time state — it is a specialized mechanism,
not a general evaluator.

## How to add a new mechanism

1. Create `mechanisms/<name>/`.
2. Add `README.md` with: purpose, inputs, algorithm, manifest shape,
   supporting infrastructure, output class (gate / anomaly / generative).
3. Implement the handler — accepts `policy_assessment.Request`
   (carrying `Facts`), returns `policy_assessment.Output` (carrying
   `RuleResult`). Rule-level signals are `[]any` (polymorphic string
   or dict).
4. Register at composition time in the caller process — the AuthZ
   runtime (PDP / `cmd/pdp`) for millisecond-budget mechanisms,
   `cmd/worker` for scan-budget mechanisms.
5. Each caller chooses its own allowlist of mechanisms to load —
   registered ≠ enabled. A mechanism whose backend doesn't fit the
   caller's SLO simply isn't on the allowlist.

