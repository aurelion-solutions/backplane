# cedar mechanism

Cedar policies (RBAC + ABAC + ReBAC in one language) for authorization
decisions. The author writes Cedar text — no Aurelion-specific YAML
wrapper, no Rego, no in-house DSL. Backend evaluator is `cedar-go`
(Apache 2.0, the Cedar Policy team's reference Go implementation),
integrated directly into this mechanism.

**Status:** active. Covers the reactive gate path
(`Decision.Effect = allow/deny`). Wired through the AuthZen transport
in `cmd/pdp`.

## When to use

- Any AuthZ decision: "can principal X do action Y on resource Z?"
- Conditional permissions: "permit if attribute matches, time window
  valid, group membership holds".
- Group / role hierarchies, resource ownership chains, parent-child
  relationships.

## When NOT to use

- Combinatorial detection over many records (use `sod`).
- Numerical scoring / risk aggregation (use `risk_scoring`).
- Anything requiring external I/O during evaluation (LLM call, DB
  query) — Cedar is pure-eval; the handler supplies all data up front.
- Graph traversal beyond Cedar's `in` containment (use `graph_analysis`).

## Inputs

- **Cedar policy** — text sibling to the manifest in the cartridge
  (`<rule>.cedar`). The handler parses and caches it in `Prepare`.
- **`Facts.Principal`** — the handler auto-builds the Cedar principal
  entity from `PrincipalFacts`: `is_active = (Status == "active")`,
  `mfa_enabled`, `tenant_id`, `email_verified` land in attrs.
- **`Facts.Resource`** (or `Facts.Target.ResourceType + .Resource` as
  a fallback) — becomes the Cedar resource UID.
- **`Facts.Action`** — becomes the `Action::"<name>"` UID.
- **`Facts.Context` + `Facts.Threat`** — flattened into the Cedar
  Context Record (`context.transport`, `context.country`,
  `context.threat.risk_score`, …).
- **`Facts.Entities`** — caller-supplied graph (groups, organisations,
  applications) for Cedar `in`-checks.

## Algorithm

1. `Prepare(entry)` — reads the sibling `.cedar` file (name taken
   from `Manifest.Body["policy_file"]` or, by default, derived from
   the manifest filename), parses it into a `cedar-go` PolicySet,
   caches it.
2. `Evaluate(req)` — builds the entity map (`buildEntities`), the
   principal/resource/action UIDs and the Context Record from
   `Facts`, then calls `policySet.IsAuthorized(entities, request)`.
3. Maps the result into `policy_assessment.Output{Result: RuleResult{Decision: ...}}`:

   | Cedar response | `Decision` | `Matched` |
   |---|---|---|
   | Allow + ≥1 Reason | `Effect=allow`, Reasons populated | true |
   | Deny + ≥1 Reason (forbid fired) | `Effect=deny`, Reasons populated | true |
   | Deny + 0 Reasons (no permit) | `Decision=nil` — policy not applicable | false |

   **Cedar's default-deny is not a verdict** — it means "this policy
   doesn't apply to this request, let the others speak". Without this
   rule a single `permit` policy plus an unrelated request would deny
   on every selection miss.

4. Cedar `diagnostic.Reasons` → `policy_assessment.Reason` objects:
   `RuleID = "<cartridge>/<rule_id>"`, `Produced =
   {cedar_policy_id, position}`.

## Manifest shape

```jsonc
{
  "rule_id":   "demo.allow_doc42_view",
  "version":   1,
  "name":      "Allow view of doc-42",
  "mechanism": "cedar",
  "severity":  "medium",
  "tags":      ["authz", "action:view", "resource:Document"],
  "body": {
    "policy_file": "allow.cedar"
  }
}
```

`body.policy_file` is optional — when absent, the handler resolves
the sibling with the same basename as `.meta.json`, replacing
`.meta.json` with `.cedar`.

The `.cedar` is plain Cedar:

```cedar
permit (
    principal,
    action == Action::"view",
    resource == Document::"doc-42"
);
```

Or with conditions:

```cedar
forbid (
    principal,
    action,
    resource
) when {
    principal.is_active == false
};
```

## Output: example

`bob` with `status=disabled` against `demo/deny_inactive` — the handler
returns:

```json
{
  "matched": true,
  "result": {
    "decision": {
      "effect": "deny",
      "reasons": [
        {
          "rule_id": "demo/deny_inactive",
          "rule_kind": "reactive_gate",
          "produced": {
            "cedar_policy_id": "policy0",
            "position": "deny.cedar:1-7"
          }
        }
      ]
    }
  }
}
```

## Related code

- `handler.go` — the handler itself (`Prepare` / `Evaluate`).
- `entities.go` (TBD) — extracted from `handler.go` if it grows too
  large.
- `handler_test.go` — three cases: Permit allow, Deny via inactive
  principal, NoMatch produces no Decision.
