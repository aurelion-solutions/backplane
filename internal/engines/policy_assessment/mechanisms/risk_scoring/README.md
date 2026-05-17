# risk_scoring mechanism

Aggregates multiple risk signals (UEBA score, ITDR alerts, geo,
time-of-day, last_seen, resource sensitivity, behavioural anomaly)
into a single numeric score, then calibrates the score to a
`RiskLevel` via threshold table.

**Output class:** reactive **anomaly** — emits `Decision.RiskLevel +
Decision.Signals` (contributing signals as structured dicts), leaves
`Decision.Effect` empty.

## When to use

- Adaptive authentication (`risk_score > 0.7` → require step-up).
- Per-action risk gating ("show me high-risk grant approvals").
- Continuous risk posture rollup.

## When NOT to use

- Binary AuthZ decisions without scoring (use `cedar`).
- Detection over a population, not a single principal (use `sod` /
  `behavioral` / `windowed_threshold`).

## Inputs

- **`Facts.Principal`** / **`Facts.Target`** from the request.
- **`Facts.Threat`** — pre-enriched risk fields (`risk_score`,
  `ueba_risk_score`, `active_indicators`, `behavioral_anomaly`,
  `failed_auth_count`, `credential_compromised`, …).
- **Signal sources** the handler gathers up front (when not already
  in `Facts.Threat`):
  - UEBA provider score (via `core/llm` or `core/ueba`).
  - ITDR detection feed.
  - Inventory facts (privilege flags, MFA state, account age).
  - `behavioral` mechanism output (anomaly signal injected as a fact).

## Algorithm

1. Collect signals from configured sources — handler runs each
   collector in parallel.
2. Apply weighted aggregation (initially: linear weights from
   manifest data; later: ML model loaded from `core/ml`).
3. Calibrate score to a `RiskLevel` (`critical` / `high` / `medium` /
   `low` / empty) via the manifest's threshold table.
4. Emit `Decision.RiskLevel` + contributing signals in
   `Decision.Signals` (each as a structured dict `{"kind": "<source>",
   "score": <n>, "weight": <w>}`).

## Manifest shape

```jsonc
{
  "rule_id":   "glyph.risk.adaptive_login",
  "version":   1,
  "name":      "Adaptive login risk scoring",
  "mechanism": "risk_scoring",
  "severity":  "medium",
  "tags":      ["authn", "risk"],
  "body": {
    "weights": {
      "ueba_score":        0.40,
      "itdr_alert_count":  0.20,
      "geo_anomaly":       0.15,
      "privilege_level":   0.15,
      "account_age_days":  0.10
    },
    "thresholds": {
      "critical": 0.90,
      "high":     0.70,
      "medium":   0.40,
      "low":      0.00
    }
  }
}
```

No external file — weights and thresholds are data, they live in the
manifest body.

## Supporting infrastructure

- Signal collectors per source — small Go interfaces, registered at
  composition time.
- Optional ML model artifact storage (out of scope for first cut).
- Confidence calibration tables.

## Status

Skeleton. Ship after `behavioral` (so we have at least one signal
source other than inventory facts).
