# sod mechanism — Segregation of Duties

Detects toxic combinations of capabilities held by one principal:
"creates POs and approves POs", "modifies vendor master and processes
payments", and similar pairs / triples described in the rule body.

The combinatorial nature of the problem is why this is its own
mechanism rather than a Cedar policy — Cedar evaluates one decision
per request; SoD enumerates conflict sets across the principal's
full capability snapshot in one go.

**Output class:** reactive **anomaly** — emits `Decision.RiskLevel` +
`Decision.Signals`, leaves `Decision.Effect` empty. Each detected
conflict surfaces as one structured entry in `Signals`
(`{"kind": "sod_conflict", ...}`) plus a string code `"sod_conflict"`.

## When to use

- Compliance reviews (SOX, GDPR access review).
- Pre-grant what-if simulations.
- Continuous scan over the principal population.

## When NOT to use

- Single-permission authorisation (use `cedar`).
- Risk scoring on a principal (use `risk_scoring`).

## Inputs

- `Facts.Principal.CapabilitySlugs` — the principal's currently-held
  capability slugs at evaluation time.

## Manifest body

```yaml
rule_id:   sod/no_self_approval
mechanism: sod
severity:  high
body:
  conditions:
    - capability_slugs: [payments.approver, payments.creator]
      min_count: 2
```

- `conditions` — one or more capability-slug groups. Each carries the
  set of slugs in scope plus a `min_count` threshold.
- `min_count` is the minimum size of the intersection between the
  principal's slugs and the condition's slug set that fires the
  condition.
- A rule fires when **every** condition fires (logical AND across
  conditions). Use multiple rules for OR.

`Prepare` validates the body (rejects empty `conditions`, empty slug
sets, `min_count <= 0`) and caches the parsed condition list keyed
by `(cartridge_ref, rule_id)`.

## Algorithm

For each condition: intersect the principal's `CapabilitySlugs` with
the condition's slug set. The condition fires when the intersection
size is `>= min_count`. The rule fires when every condition fires.

When the rule fires, the handler emits a single `Decision`:

- `RiskLevel` — pulled from the rule's `severity`.
- `Signals` — `["sod_conflict", {"kind": "sod_conflict", "principal": "...", "conditions": [{"required": [...], "min_count": N, "matched": [...]}, ...]}]`.
- `Reasons` — one entry with `RuleID`, `RuleKind: "anomaly"`, and the
  matched intersection per condition.

When the rule does not fire the handler returns `Matched=false` with
`Result.Decision = nil`.

## Scope

This revision is **global**: conditions evaluate over the principal's
entire `CapabilitySlugs` set with no per-application or per-scope
slicing. Per-grant scope context is not yet on `Facts`, so the
`scope_mode` field intentionally does not exist on the manifest.

## Output: example

Principal `alice` holds both `payments.approver` and
`payments.creator`:

```json
{
  "matched": true,
  "result": {
    "decision": {
      "risk_level": "high",
      "signals": [
        "sod_conflict",
        {
          "kind": "sod_conflict",
          "principal": "alice",
          "conditions": [
            {
              "required": ["payments.approver", "payments.creator"],
              "min_count": 2,
              "matched": ["payments.approver", "payments.creator"]
            }
          ]
        }
      ],
      "reasons": [
        {
          "rule_id": "sod/no_self_approval",
          "rule_kind": "anomaly",
          "matched_conditions": {
            "principal.capability_slugs ⊇": ["payments.approver", "payments.creator"]
          }
        }
      ]
    }
  }
}
```

The assess action persists this as a `findings` row with
`severity = high` and `signals = decision.signals` verbatim.
