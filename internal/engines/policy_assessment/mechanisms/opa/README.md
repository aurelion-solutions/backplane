# opa mechanism

Rego-based predicate evaluator over `Facts`. Used for the two classes
of rules where Cedar's "can principal do action on resource" framing
is unnatural:

- **Reactive anomaly findings** — orphan accounts, dormant privileged
  accounts, terminated subjects still holding access, "last_login >
  1y ago" patterns, and similar fact-derived detections.
- **Generative rules** — birthright joiner projections, leaver
  revocations, grace windows. Rule emits `projected_facts[]` directly,
  Connector takes the diff against current state.

Backend evaluator is `github.com/open-policy-agent/opa` embedded
(in-process `rego.PreparedEvalQuery`) — no OPA daemon, no sidecar.

**Output class:** anomaly (`Decision.RiskLevel + Decision.Signals`)
and/or generative (`ProjectedFacts[]`). Same handler covers both —
the rule body decides which output variable it fills.

## When to use

- Predicate over a single inventory record:
  "this account has `principal_id == null` and `last_login_at > now - 1y`".
- Predicate over a small derived set:
  "principal is `terminated` AND still has any `account_status == 'active'` grant".
- Generative projection:
  "principal in `department == 'R&D'` gets birthright access to jira + slack".
- Grace-window emission:
  "principal `terminated` < 3 days ago — emit projected_fact with
  `valid_until = term_date + 3d`".

## When NOT to use

- AuthZ decisions ("can principal X do Y on resource Z?") — that's
  `cedar`. Cedar is type-checked and formally verifiable; Rego is
  general-purpose and Aurelion doesn't want hand-rolled AuthZ logic
  drifting between cartridges.
- Combinatorial enumeration over many records (SoD pairs / triples) —
  that's `sod` (DB-backed evaluator, faster and persisted rules).
- Weighted numerical aggregation — that's `risk_scoring`.
- Anything requiring external I/O during evaluation (LLM call, DB
  query, HTTP fetch) — Rego is pure-eval, the caller supplies all
  input up front.

## Inputs

- **Rego policy** — text file sibling-to-manifest in the cartridge
  (`<rule>.rego`). Handler parses + compiles in `Prepare`, caches
  the `rego.PreparedEvalQuery`.
- **`Facts`** — the canonical PDP input. Handler passes it to Rego
  as `input` verbatim (serialized to JSON). Rule body reads
  `input.principal.*`, `input.target.*`, `input.context.*`, etc.

Common Rego paths the rule body reads:

```
input.principal.id, input.principal.status, input.principal.kind, …
input.target.application, input.target.kind, input.target.principal_id, …
input.action, input.context.transport, input.context.country, …
input.threat.risk_score, input.threat.behavioral_anomaly, …
input.now            # ISO 8601 string; use time.parse_rfc3339_ns
```

## Algorithm

1. **Prepare(entry)** — read `<rule>.rego` (default name = manifest
   basename with `.rego` extension; override via
   `body.policy_file`), call `rego.New(...).PrepareForEval(ctx)`,
   cache the prepared query keyed by `<cartridge>/<rule_id>`.
2. **Evaluate(req)** — `query.Eval(ctx, rego.EvalInput(facts))`.
   The query is shaped so the result yields the two output variables
   the rule defines:
   - `data.<package>.decision` — when present and non-null →
     populate `RuleResult.Decision`.
   - `data.<package>.projected_facts` — when present and non-empty →
     populate `RuleResult.ProjectedFacts`.
3. **Result mapping:**

   | Rego output | `Output.Result` |
   |---|---|
   | `decision = {...}` non-null | `Decision` populated (Effect / RiskLevel / Signals / Reasons mapped 1:1) |
   | `projected_facts = [...]` non-empty | `ProjectedFacts` populated |
   | both | both populated (hybrid; allowed but rare) |
   | neither | `Matched = false`, `Decision = nil`, `ProjectedFacts = nil` — "rule not applicable" |

   Rego rule errors propagate to `error` from `Evaluate`; the dispatcher
   logs and the caller decides what to do (policy-assessment action:
   skip + continue; AuthZ transport: deny-wins fallback or 500).

4. **Signal polymorphism preserved.** Rego `"signals": ["foo", {"kind": "bar", ...}]`
   is deserialized into `[]any` verbatim — no struct flattening. A
   signal is either a string code or a structured dict; both shapes
   coexist in the same list.

## Manifest shape

```jsonc
{
  "rule_id":   "lens.access_risk.orphaned_account_recent_login",
  "version":   1,
  "name":      "Orphaned account, recent login",
  "mechanism": "opa",
  "severity":  "high",
  "tags":      ["scan", "anomaly", "framework:sox"],
  "body": {
    "policy_file": "orphaned_account_recent_login.rego"
  }
}
```

`body.policy_file` is optional — when absent, the handler resolves
the sibling with the same basename as `.meta.json`, replacing
`.meta.json` with `.rego`.

The `.rego` body fills the `decision` / `projected_facts` variables
defined by the rule contract:

```rego
package lens.access_risk.orphaned_account_recent_login

import rego.v1

year_ns := 365 * 24 * 3600 * 1000000000

default decision := null

decision := {
    "risk_level": "high",
    "signals":    ["orphaned_account_recent_login"],
    "reasons": [{
        "rule_id": "lens.access_risk.orphaned_account_recent_login",
        "rule_kind": "anomaly",
        "matched_conditions": {
            "target.principal_id":   "null",
            "target.last_login_at":  "> now - 1y"
        },
        "fact_values": {
            "target.principal_id":  input.target.principal_id,
            "target.last_login_at": input.target.last_login_at
        }
    }]
} if {
    input.target.kind == "account"
    input.target.principal_id == null
    input.target.last_login_at != null
    time.parse_rfc3339_ns(input.target.last_login_at) > time.now_ns() - year_ns
}
```

Generative birthright example:

```rego
package journey.birthright.rd_employee

import rego.v1

default projected_facts := []

projected_facts := base if {
    input.principal.status == "active"
    input.principal_context.attributes.department == "R&D"
}

base := [
    {
        "target":        {"application": "jira",  "resource_type": "account", "resource": "primary"},
        "initiative":    "birthright",
        "desired_state": {"present": true, "attributes": {}},
        "signals":       ["birthright_rd_jira"],
        "reasons": [{
            "rule_id":            "journey.birthright.rd_employee",
            "rule_kind":          "generative_birthright",
            "matched_conditions": {"principal_context.attributes.department": "R&D"}
        }]
    },
    {
        "target":        {"application": "slack", "resource_type": "account", "resource": "primary"},
        "initiative":    "birthright",
        "desired_state": {"present": true, "attributes": {}},
        "signals":       ["birthright_rd_slack"],
        "reasons": [{
            "rule_id":            "journey.birthright.rd_employee",
            "matched_conditions": {"principal_context.attributes.department": "R&D"}
        }]
    }
]
```

## Output: example

`orphan_accounts.rego` against account `alice@example.com` with no
owner and a recent login:

```json
{
  "matched": true,
  "result": {
    "decision": {
      "risk_level": "high",
      "signals": ["orphaned_account_recent_login"],
      "reasons": [
        {
          "rule_id": "lens.access_risk.orphaned_account_recent_login",
          "rule_kind": "anomaly",
          "matched_conditions": {
            "target.principal_id":  "null",
            "target.last_login_at": "> now - 1y"
          },
          "fact_values": {
            "target.principal_id":  null,
            "target.last_login_at": "2026-04-12T08:15:00Z"
          }
        }
      ]
    }
  }
}
```

The policy-assessment action persists this as a `findings` row:
`severity = high` (from the manifest), `risk_level = high` (from the
decision), `signals` and `reasons` are stored verbatim.

## Supporting infrastructure

- `github.com/open-policy-agent/opa` (embedded SDK — `rego` package).
  Not a sidecar / not a daemon — in-process eval.
- Cartridge `Provider.Policies()` already supplies `Manifest.BasePath`
  → handler resolves sibling `.rego` from there (same convention as
  `cedar` resolves `.cedar`).
- No DB tables. Rules and bodies live in cartridges; the
  `inventory/policies` mirror only stores metadata.

## Status

Wired. `handler.go` ships `Prepare` / `Evaluate` over an embedded
`rego.PreparedEvalQuery`; the package path is resolved via
`ast.ParseModule` of the loaded module. Unit tests cover allow gate,
anomaly with polymorphic signals, generative birthright, no-match,
and the explicit `body.policy_file` override.
