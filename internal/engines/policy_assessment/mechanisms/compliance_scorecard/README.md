# compliance_scorecard mechanism

Rollup metric: aggregates many sub-findings into one scorecard value
per framework (SOX / PCI / GDPR / ISO 27001 / SOC 2). Output is
"X% compliant for framework Y" plus the contributing failures.

## When to use

- Periodic compliance posture reports.
- Auditor-facing dashboards.
- "What's our SOX compliance after the last reorg?"

## When NOT to use

- Per-control evaluation — that lives in the underlying mechanisms
  (`cedar` / `sod` / `behavioral`) whose findings this rollup consumes.

## Inputs

- **Framework definition** — control set the scorecard reports on.
- **Recent findings** — pre-computed by other mechanisms, queried
  from the findings store.
- **Mitigation state** — accepted risks / approved exceptions.

## Algorithm

1. Pull all controls under the framework.
2. Per control, query the findings store for the most recent verdict.
3. Weight by control severity, apply mitigation overrides.
4. Emit a scorecard with per-control breakdown.

**Output class:** anomaly **rollup** — emits `Decision.RiskLevel`
calibrated from the percentage and a list of `Decision.Signals`
(structured dicts with `{"kind": "control_breakdown", "control_id":
"...", "verdict": "pass|fail|waived"}`).

## Manifest shape

```jsonc
{
  "rule_id":   "glyph.compliance.sox_quarterly",
  "version":   1,
  "name":      "SOX quarterly scorecard",
  "mechanism": "compliance_scorecard",
  "severity":  "info",
  "tags":      ["scan", "compliance", "framework:sox"],
  "body": {
    "framework": "SOX",
    "controls": [
      "glyph.sod.po_creator_approver",
      "glyph.cedar.privileged_access_review",
      "glyph.behavioral.admin_login_pattern"
    ],
    "weighting": "uniform"
  }
}
```

## Supporting infrastructure

- Findings store (M5) — must exist before this mechanism is useful.
- Framework control definitions — admin-managed catalog.

## Status

Placeholder. Reasonable to ship once findings persistence (M5) and a
handful of underlying mechanisms are live.
