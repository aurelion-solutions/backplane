# policy_assessment.actions

Orchestrator entry points into the policy-assessment engine. One
sub-package per action.

| Action | Pair | Purpose |
|---|---|---|
| `assess` | `("policy_assessment", "assess")` | Walk the account population through every applicable policy and write findings keyed by `evidence_hash`. |
