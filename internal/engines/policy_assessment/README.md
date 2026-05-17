# policy_assessment engine

The single dispatch axis for **every** policy evaluation in Aurelion:
AuthN, AuthZ, SoD, generative birthright/leaver, risk scoring,
behavioural anomalies, LLM classification, graph analysis, compliance
scorecard, quorum, windowed thresholds.

**One engine, one dispatcher, N mechanism handlers.** Each handler
closes one class of evaluation end-to-end: prepares the policy once
(`Prepare`) and evaluates it many times (`Evaluate`). The caller
(PDP HTTP transport, the policy-assessment action running in
`cmd/worker`) does not know which mechanism runs — it builds `Facts`,
dispatches through the engine, aggregates the resulting `RuleResult`s.

The rule contract is `RuleResult` — `Decision` (reactive verdict)
plus `ProjectedFacts` (generative output) over the canonical
`Facts` envelope. Cedar / graph mechanisms also see `Resource` and
`Entities` for their own transport needs.

---

## Layout

```
policy_assessment/
├── contracts.go          — Request / Output / AssessmentSignal / Evidence
├── schemas.go            — Facts / RuleResult / Decision / ProjectedFact / Reason
├── dispatcher.go         — Handler interface + routing
├── store.go              — in-memory snapshot, tag-based pre-filter
├── store_watcher.go      — polls cartridges.Provider, swaps snapshot
└── mechanisms/
    ├── cedar/                — Cedar policies — AuthZ (RBAC/ABAC/ReBAC)
    ├── opa/                  — Rego predicates — anomaly + generative
    ├── sod/                  — Segregation of Duties (DB-backed combinatorial)
    ├── risk_scoring/         — weighted signals → score → threshold
    ├── behavioral/           — baseline / anomaly detection
    ├── llm_classification/   — LLM prompt + structured output
    ├── graph_analysis/       — toxic-path / cycle / closure traversal
    ├── compliance_scorecard/ — rollup metrics
    ├── quorum/               — N-of-M approval gating
    └── windowed_threshold/   — time-windowed breach detection
```

---

## Universal rule contract

One shape per rule, per mechanism. Input — `Facts`. Output —
`RuleResult`.

### The heart: `RuleResult`

```go
type RuleResult struct {
    Decision       *Decision        // reactive verdict (gate / anomaly)
    ProjectedFacts []ProjectedFact  // generative output (birthright / leaver / grace)
}
```

The rule's class decides which fields populate:

| Rule class                | `Decision`            | `ProjectedFacts` |
|---------------------------|-----------------------|------------------|
| reactive **gate**         | `Effect=allow/deny`   | empty            |
| reactive **anomaly**      | `RiskLevel + Signals`, `Effect` empty | empty |
| generative birthright/leaver | nil                | non-empty        |
| hybrid (rare)             | both                  | both             |

### `Decision`

```go
type Decision struct {
    Effect    string    // "allow" / "deny" — gates only
    RiskLevel string    // "critical"/"high"/"medium"/"low" — anomaly / risk
    Signals   []any     // polymorphic: string code OR structured dict
    Reasons   []Reason  // audit trail — which condition fired
}
```

**`Effect` and `RiskLevel` are orthogonal.** An anomaly finding leaves
`Effect` empty and populates `RiskLevel`+`Signals`. An AuthZ gate
populates `Effect` and leaves `RiskLevel` empty.

`Signals` is a **polymorphic list of markers** — each entry is
either a string code or a structured dict:

- `"orphaned_account_recent_login"` — plain string code for UI / findings
- `{"kind": "sod_conflict", "role_a": "...", "role_b": "..."}` — a structured finding; consumers branch on `kind`

### `Reason`

```go
type Reason struct {
    RuleID            string         // "<cartridge>/<rule_id>"
    RuleKind          string         // "reactive_gate", "anomaly", "generative_birthright", …
    Precedence        int
    MatchedConditions map[string]any // which conditions fired (audit)
    FactValues        map[string]any // which facts triggered those conditions
    Produced          map[string]any // mechanism-specific payload (e.g. cedar policy id + position)
}
```

Reconstruction-friendly: any decision can be explained via
`RuleID + MatchedConditions + FactValues`. This lands in
`findings.reason_chain` and in the audit log.

### `ProjectedFact`

```go
type ProjectedFact struct {
    Target       TargetFacts     // what we project access to
    Initiative   string          // "birthright" / "requested" / "delegated" / "grace"
    ValidFrom    *time.Time      // when it becomes effective
    ValidUntil   *time.Time      // when it expires (grace, joiner-ramp)
    DesiredState DesiredState    // present=true → grant; present=false → revoke
    RiskLevel    string
    Signals      []any
    Reasons      []Reason
}

type DesiredState struct {
    Present    bool             // true: "this access must exist"; false: "this access must NOT exist"
    Attributes map[string]any   // target attribute set (roles, scope, limits)
}
```

**The model is declarative** — no actions/grant/revoke in the API.
The connector diffs `CurrentFacts ⨯ ProjectedFacts` and applies the
minimum change set that converges on `DesiredState`. When
`valid_until` expires and the rule stops emitting the projected fact,
the diff shows "projected empty, current present" → revoke.

---

## Three scenarios

### 1. Reactive gate (Cedar / AuthZ)

`bob` with `status=disabled` triggers `demo/deny_inactive`:

```json
{
  "decision": {
    "effect": "deny",
    "risk_level": null,
    "signals": ["inactive_principal_denied"],
    "reasons": [
      {
        "rule_id": "demo/deny_inactive",
        "rule_kind": "reactive_gate",
        "matched_conditions": {"principal.is_active": false, "action": "view"},
        "fact_values": {"principal.id": "bob", "principal.status": "disabled"},
        "produced": {"cedar_policy_id": "policy0", "position": "deny.cedar:1-7"}
      }
    ]
  },
  "projected_facts": []
}
```

### 2. Reactive anomaly finding (SoD)

`alice` holds two conflicting capability slugs — no allow/deny, just
a finding. `signals` mixes a string marker with a structured dict:

```json
{
  "decision": {
    "effect": null,
    "risk_level": "high",
    "signals": [
      "sod_conflict",
      {
        "kind": "sod_conflict",
        "role_a": "payments.approver",
        "role_b": "payments.creator",
        "principal": "alice",
        "application": "ERP"
      }
    ],
    "reasons": [
      {
        "rule_id": "sod/no_self_approval",
        "rule_kind": "anomaly",
        "matched_conditions": {
          "principal.capability_slugs ⊇": ["payments.approver", "payments.creator"]
        },
        "fact_values": {
          "principal.id": "alice",
          "principal.capability_slugs": ["payments.approver", "payments.creator", "general.read"]
        }
      }
    ]
  },
  "projected_facts": []
}
```

The policy-assessment action turns this into a `findings` row:
`severity = decision.risk_level`, `signals = decision.signals`
verbatim (polymorphic), `reasons` go into the audit payload.

### 3. Generative — R&D joiner in NYC + Jira grace

A single rule in a single cartridge can emit several `projected_facts`:

```json
{
  "decision": null,
  "projected_facts": [
    {
      "target": {"application": "jira", "resource_type": "account", "resource": "primary"},
      "initiative": "birthright",
      "valid_from": null,
      "valid_until": null,
      "desired_state": {"present": true, "attributes": {}},
      "risk_level": null,
      "signals": ["birthright_rd_jira"],
      "reasons": [{
        "rule_id": "journey/birthright.rd_employee",
        "rule_kind": "generative_birthright",
        "matched_conditions": {
          "principal.status": "active",
          "principal_context.attributes.department": "R&D"
        },
        "fact_values": {
          "principal.id": "emp-77",
          "principal_context.attributes.department": "R&D"
        }
      }]
    },
    {
      "target": {"application": "slack", "resource_type": "channel_membership", "resource": "we_are_new_yorkers"},
      "initiative": "birthright",
      "desired_state": {"present": true, "attributes": {}},
      "signals": [{"kind": "geo_extra", "location": "NYC"}],
      "reasons": [{
        "rule_id": "journey/birthright.rd_employee",
        "matched_conditions": {"principal_context.attributes.location": "NYC"}
      }]
    },
    {
      "target": {"application": "jira", "resource_type": "account", "resource": "primary"},
      "initiative": "grace",
      "valid_from": "2026-05-28T09:00:00Z",
      "valid_until": "2026-05-31T09:00:00Z",
      "desired_state": {"present": true, "attributes": {}},
      "signals": [
        "leaver_jira_grace",
        {"kind": "grace_window", "days": 3, "until": "2026-05-31T09:00:00Z"}
      ],
      "reasons": [{
        "rule_id": "journey/leaver.jira",
        "rule_kind": "generative_grace",
        "matched_conditions": {"principal.status": "terminated"}
      }]
    }
  ]
}
```

---

## `Facts` — canonical PDP input

`Facts` is the only envelope a handler sees. The caller (PDP transport
/ policy-assessment action) populates whatever sections its scenario needs; the
rule reads only what it cares about.

```go
type Facts struct {
    Principal          *PrincipalFacts          // actor: Account / Person / NHI / Workload
    Target             *TargetFacts             // access object (policy-driven access)
    Action             string                   // "view", "approve", "delete", …
    Resource           *Resource                // REST/Cedar object (Type + ID + Properties) — Go extension
    Context            *ContextFacts            // transport / country / IP / extras
    PrincipalContext   *PrincipalContextFacts   // org_unit_id + extended attrs (generative)
    CurrentFacts       []map[string]any         // current access snapshot (for projection diff)
    CurrentInitiatives []map[string]any         // current initiatives on the target
    Threat             *ThreatFacts             // risk_score, UEBA, indicators, behavioral_anomaly
    Entities           []EntityRecord           // Cedar entity JSON: uid/attrs/parents — Go extension
    Now                time.Time                // required
    Extra              map[string]any           // caller-supplied facts outside the typed sections
}
```

### `PrincipalFacts`

ID + Kind + Status + OrgUnit + StartDate/TermDate + NHI Owner +
TenantID/TenantRole/TenantStatus + EmailVerified + MFAEnabled +
PlanTier + CapabilitySlugs + Attributes. Used for every actor type —
human employees, NHIs, tenant users.

> **Terminology.** Aurelion calls the actor `principal` everywhere.
> The AuthZen wire format uses `subject`; the transport layer
> translates `req.Subject` → `Facts.Principal` in `buildFacts`.

### `TargetFacts`

Application + Kind + ResourceType + Resource + ID + PrincipalID +
AccountStatus/AccountIsPrivileged + LastLoginAt + Initiatives +
PrivilegeLevel + Environment + DataSensitivity + pending attestations.
Describes "what" and "how sensitive".

### `ContextFacts`

Transport (`saml` / `oidc` / `passkey` / …), Country, IP, plus a free
`Extra` for any other request parameters.

### `ThreatFacts`

`risk_score`, `active_indicators[]`, `days_since_last_login`,
`days_since_last_use`, `failed_auth_count`, `credential_compromised`,
`ueba_risk_score`, `behavioral_anomaly`. Feeds `risk_scoring`,
`behavioral`, and parts of Cedar `when` clauses.

### `Resource` and `EntityRecord`

Two complementary shapes for the access object:

- `Resource{Type, ID, Properties}` — simple shape, convenient for REST AuthZ.
- `EntityRecord{UID, Attrs, Parents}` — graph shape, maps 1-to-1 to Cedar entity JSON.

Callers may supply both; each mechanism picks the form that fits.

---

## Engine-level wrapper: `Output`

`RuleResult` is the **rule** contract. `Output` is the **dispatcher**
contract — it carries a `RuleResult` plus engine-side diagnostics:

```go
type Output struct {
    Matched     bool                  // policy is applicable to the request
    Result      RuleResult            // canonical rule result
    Signals     []AssessmentSignal    // engine-level telemetry (NOT rule signals)
    Evidence    []Evidence            // audit support
    Explanation string                // human-readable summary
    Confidence  *float64              // [0,1] when the mechanism can estimate
    Payload     map[string]any        // debug/trace, outside the stable contract
}
```

**Two different Signal concepts.**

| Where | Type | Purpose |
|---|---|---|
| `Decision.Signals` / `ProjectedFact.Signals` | `[]any` (polymorphic: string OR dict) | Rule-level. What the rule emits per the canonical contract. Polymorphic so Rego/Cedar/SoD can emit `"foo"` or `{"kind":"bar"}` without wrapper code. |
| `Output.Signals` | `[]AssessmentSignal{Code, Severity, Message, Payload}` | Engine-level. A mechanism handler may normalize rule signals into a typed shape for dispatcher telemetry. Not part of the rule contract. |

---

## Tag-based pre-filter

Before the mechanism handler runs, the store filters for **applicable
policies** via subset tag matching.

### Policy manifest

```jsonc
{
  "rule_id":   "demo.allow_doc42_view",
  "version":   1,
  "name":      "Allow view of doc-42",
  "mechanism": "cedar",
  "tags":      ["authz", "action:view", "resource:Document"]
}
```

### Request (transport)

The transport derives `facets[]` from its envelope — for AuthZen:

```go
deriveFacets(req) = []string{
    "authz",
    "action:view",
    "resource:Document",
    "principal:Account",
    "context.transport:oidc",
    "context.country:DE",
}
```

### Selection rule

```
policy.tags ⊆ request.facets
```

Every tag on the policy must be in the request's facet set. A policy
with no tags is treated as "applicable everywhere". Facets are opaque
strings — semantics (`geo:eu` vs `geo:DE`) are the caller's
responsibility: if you want `DE` requests to match an `EU` policy,
emit both facets.

---

## Policy lifecycle

```
        ┌──── cartridges.Provider (FS / OCI) ────┐
        │   .meta.json + sibling artefact         │
        │   (.cedar / .rego / .prompt / .yaml)    │
        └────────────────┬────────────────────────┘
                         │
                         ▼
         store_watcher.go (poll)
                         │
                         ▼
         Store.Reload(provider) — atomic snapshot swap
                         │
                         ▼
         dispatcher.PrepareAll(entries)
            └─ for every entry: handler.Prepare(ctx, entry)
               (cedar parses .cedar, opa compiles .rego,
                llm compiles prompt template,
                sod loads rule sets from PG)
                         │
                         ▼
                  ready to Evaluate
```

`Prepare` runs the one-time expensive work (parse / compile / preload).
`Evaluate` is goroutine-safe, runs many times, and avoids I/O beyond
what is strictly required. A `Prepare` failure drops the entry from
the snapshot — the rest of the catalogue stays live.

---

## Request flow

```
┌───────────────────────────────┐
│ HTTP transport /              │
│ policy-assessment action      │
│ (PDP AuthZen, SAML, OIDC, …)  │
└──────────────┬────────────────┘
               │ build Facts from envelope (req.Subject → Facts.Principal)
               ▼
┌───────────────────────────────┐
│ derive facets                 │  ["authz","action:view","resource:Document","principal:Account",…]
└──────────────┬────────────────┘
               ▼
┌───────────────────────────────┐
│ store.SelectByFacets(facets)  │  → []Entry (filtered policies)
└──────────────┬────────────────┘
               │ per Entry
               ▼
┌───────────────────────────────┐
│ dispatcher.EvaluateEntry      │
│   → handlers[mechanism]       │
│       .Evaluate(Request{Facts})│
└──────────────┬────────────────┘
               ▼
┌───────────────────────────────┐
│ per-policy Output[]           │  (each Output carries a RuleResult)
└──────────────┬────────────────┘
               ▼
┌───────────────────────────────┐
│ caller aggregates             │
│  — AuthZ: deny-wins over      │
│    Decision.Effect            │
│  — assessment: append findings│
│  — risk: max score            │
└──────────────┬────────────────┘
               ▼
         transport response
```

Aggregation belongs to the **caller**, not the engine. The AuthZ PDP
applies deny-wins over `Decision.Effect`; the policy-assessment
action persists every matched output as a finding row; risk_scoring
sums scores.

---

## Cedar handler semantics (as a reference)

Cedar returns `(decision bool, diagnostic)`. By default "no permit
fired" reads as deny — but that is not a verdict, just "not
applicable". The mapping:

| Cedar response                   | `Decision`                                  | `Matched` |
|----------------------------------|---------------------------------------------|-----------|
| Allow + ≥1 Reason                | `Effect=allow`, Reasons populated           | true      |
| Deny + ≥1 Reason (forbid fired)  | `Effect=deny`, Reasons populated            | true      |
| Deny + 0 Reasons (no permit)     | `Decision=nil` — policy not applicable      | false     |

Cedar's default-deny is **not** turned into a real deny — it is
interpreted as "this policy doesn't apply to this request, let other
policies speak".

The Cedar handler auto-builds the `principal` entity from
`PrincipalFacts`: `is_active = (Status == "active")`, `mfa_enabled`,
`tenant_id`, `email_verified` land in attrs. `ThreatFacts` and
`ContextFacts` flatten into the Cedar Context Record.

---

## What this engine does **NOT** do

- **Policy persistence.** That's `inventory/policies` — the PG mirror
  of cartridge manifests.
- **Cartridge format and loading.** That's `core/cartridges`.
- **Findings persistence.** That's `inventory/findings`.
- **Scan-run orchestration.** That's the assess action in `actions/assess/`
  driven by the worker.
- **LLM providers / RAG.** That belongs in a separate `core/llm`
  package once any mechanism actually needs it.
- **Transports.** AuthZen / SAML / OIDC / Passkey live in `cmd/pdp/transport/*`.
- **Aggregation.** The caller owns its own semantics.
- **Obligations.** Not a field on `Decision`. The AuthZen response
  shape supports them on the wire; the transport populates them by
  extracting structured signals from `Decision.Signals` (entries like
  `{"kind": "obligation", ...}`).

---

## Related packages

- `internal/core/cartridges` — Manifest + FilesystemProvider + Watcher.
- `internal/inventory/policies` — PG mirror of policies.
- `cmd/pdp/transport/authzen` — first transport (AuthZen 1.0).
- `cmd/pdp/transport/...` — future transports (SAML / OIDC / Passkey / Federation).
- `internal/engines/policy_assessment/mechanisms/cedar` — Cedar
  (AuthZ). `mechanisms/opa` and `mechanisms/sod` are also wired.
  The remaining mechanism directories carry README contracts but no
  handler code yet.
