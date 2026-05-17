# behavioral mechanism

Baseline / anomaly detection over per-principal activity. Emits an
anomaly signal that downstream mechanisms (`risk_scoring`, `cedar` gate
reading `Facts.Threat.behavioral_anomaly`) can consume.

**Output class:** reactive **anomaly** — emits `Decision.RiskLevel +
Decision.Signals`, leaves `Decision.Effect` empty.

## When to use

- "User typically logs in at 9-18 from EU; this is 03:00 from RU."
- "Service account averages 50 API calls/min; current 5000/min."
- "First-time access to this resource for this principal."

## When NOT to use

- Static rules ("admin from non-corp IP" — use `cedar`).
- Risk score aggregation across multiple signals (use `risk_scoring`,
  which can take this mechanism's output as one of its signals).

## Inputs

- **Principal** under analysis.
- **Activity stream** — sourced from inventory facts / SIEM / event
  bus. Handler aggregates into per-principal features.
- **Baseline store** — historical statistics per principal (DB-persisted).

## Algorithm

1. Load principal baseline (sliding window — last N days).
2. Extract current-period feature vector from recent activity.
3. Score feature vector against baseline (z-score, IQR, learned
   model — TBD).
4. Emit signal with anomaly level + contributing features.

## Manifest shape

```jsonc
{
  "rule_id":   "glyph.behavioral.login_pattern",
  "version":   1,
  "name":      "Login pattern anomaly",
  "mechanism": "behavioral",
  "severity":  "medium",
  "tags":      ["authn", "anomaly"],
  "body": {
    "features": ["hour_of_day", "day_of_week", "country", "device_fingerprint"],
    "baseline": {"window_days": 30, "source": "inventory_facts"},
    "thresholds": {"high": 3.0, "medium": 2.0}
  }
}
```

## Supporting infrastructure

- Baseline DB tables (per-principal feature statistics, sliding window).
- Activity stream consumer (event subscriber writing into baseline
  store).
- Optional ML model store (future).

## Status

Skeleton. Lots of moving parts (baseline persistence, stream
consumer); ships after the basic mechanisms (`cedar`, `sod`) and
infrastructure (`risk_scoring`) are in place.
