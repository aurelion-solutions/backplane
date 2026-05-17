# quorum mechanism

N-of-M approval gating. A pending action becomes effective only when
N approvers from a designated approver set have signed off; less than
N → deny / hold.

**Output class:** reactive **gate** when quorum is met; otherwise
emits `Decision.Effect=""` + an `awaiting_quorum` signal (not a
verdict — just "not yet").

## When to use

- Sensitive access grants requiring co-sign (admin role assignment,
  break-glass).
- Mitigation acceptance requiring committee approval.
- Production deploy approvals (if Glyph is wired into that flow).

## When NOT to use

- Single-approver flows — use `cedar` with an `approver` attribute.
- Time-decaying approvals — use `windowed_threshold`.

## Inputs

- **Pending request** (`Facts.Principal` + `Facts.Resource` + `Facts.Action`).
- **Approval set** — manifest declares "any N of {approver list}".
- **Existing approvals** — DB-persisted votes for the pending request
  (carried via `Facts.Extra.approvals` or fetched by handler).

## Algorithm

1. Look up existing approvals for the pending request id.
2. Filter approvals to those from members of the approver set + still
   valid (not revoked, not expired).
3. Decision:
   - count ≥ N → `Decision.Effect = "allow"`
   - 0 < count < N → `Decision.Effect = ""`, signal `"awaiting_quorum"`
   - count = 0 → `Decision.Effect = ""`, signal `"quorum_required"`

## Manifest shape

```jsonc
{
  "rule_id":   "glyph.quorum.break_glass",
  "version":   1,
  "name":      "Break-glass quorum",
  "mechanism": "quorum",
  "severity":  "critical",
  "tags":      ["authz", "quorum"],
  "body": {
    "quorum": {
      "required":           2,
      "approvers_role":     "Role::\"SecurityCommittee\"",
      "approval_ttl_hours": 24
    }
  }
}
```

## Supporting infrastructure

- DB table for pending requests + their approvals.
- REST endpoint for cast / revoke approval.

## Status

Placeholder. Useful when Glyph starts handling sensitive workflows;
not first-cut.
