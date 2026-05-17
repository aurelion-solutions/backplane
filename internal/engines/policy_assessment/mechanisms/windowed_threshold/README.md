# windowed_threshold mechanism

Time-window breach detection. "More than N events of type X within
window W" → fires. Stateless from the request's perspective; the
handler counts events from a stream and the policy specifies the
threshold.

**Output class:** reactive **anomaly** — emits `Decision.RiskLevel +
Decision.Signals = ["threshold_breached", {"kind": ..., "count": N,
"window": "10m"}]`, leaves `Decision.Effect` empty.

## When to use

- "5 failed logins in 10 minutes" → lockout candidate.
- "Privileged session > 2h without re-auth" → step-up required.
- "10 grant revocations in 1h" → mass-departure alert.
- Velocity-based fraud signals.

## When NOT to use

- Persistent anomalies (use `behavioral`).
- Single-event gates (use `cedar`).

## Inputs

- **Event stream** — handler subscribes to a configured topic /
  datastore.
- **Window descriptor** — duration + sliding vs tumbling vs fixed.
- **Threshold** — `count > N`, `rate > R per minute`, etc.

## Algorithm

1. Maintain a sliding counter per `(principal, event_type)` —
   implementation can be in-memory + persisted snapshots, or a Redis
   counter, or a windowed SQL view (TBD).
2. On every counter update, check threshold.
3. When threshold crosses upward, emit `Decision.RiskLevel +
   Decision.Signals = ["threshold_breached", {...}]`.

## Manifest shape

```jsonc
{
  "rule_id":   "glyph.velocity.failed_login_burst",
  "version":   1,
  "name":      "Failed login burst",
  "mechanism": "windowed_threshold",
  "severity":  "high",
  "tags":      ["authn", "anomaly"],
  "body": {
    "event_type": "auth.login.failed",
    "window":     {"kind": "sliding", "duration": "10m"},
    "threshold":  {"count": 5}
  }
}
```

## Supporting infrastructure

- Event stream consumer (MQ subscriber or DB poll).
- Counter store (in-memory + persisted, or Redis).

## Status

Placeholder. Wire when there's a real velocity-based use case (likely
adaptive authentication / fraud signalling).
