# compliance_projection

Projects identity-posture findings onto external compliance control
languages (SOC 2 logical access, …). A projection is a **view** over a
single assessment run — never a source of truth, and never a policy
engine. This package owns **no** policies: it reads a declarative
projection definition from a cartridge and rolls the run's *existing*
findings and policy-evaluation outcomes up into per-control coverage.

> Compliance is a projection over identity truth, not the foundation.
> One finding ("privileged account without MFA") projects onto many
> controls; adding a projection is a new mapping, never a new evaluator.

## Read-time, persists nothing

Every call recomputes from the run. There is no projection table, no
materialised coverage row, no migration. Coverage is a pure function of
`(projection definition, run findings, run outcomes)`, so the surface is
trivially safe to prefetch / revalidate.

## Coverage state machine

A control is `covered` only on **positive** evidence — the population
was evaluated and produced neither a violation nor a gap. Absence of a
violation over an *unevaluated* population is `not_evaluable`, never a
silent green tick.

```text
population never reached (no outcome on any of the control's rules) → not_evaluable
violations > 0  AND  gaps == 0                                       → failed
violations > 0  AND  gaps  > 0                                       → partial
violations == 0 AND  gaps  > 0                                       → not_evaluable
violations == 0 AND  gaps == 0 (evaluated)                           → covered
```

- **violations** — baseline findings (`last_seen_run_id` = the run)
  whose `kind` the control flags as violating.
- **evaluated** — at least one rule emitting one of the control's
  violating kinds produced any outcome this run (the population was
  reached).
- **gaps** — those rules that came back `not_evaluable` (reached but
  blind). The blind spot is attributed precisely via `rule_id → kind`,
  read from the cartridge manifests — not guessed from a target type.

## Projection cartridge contract

A projection cartridge places a single `projection.json` at its root and
carries **no** `policies/` of its own:

```jsonc
{
  "projection": "soc2-logical-access",
  "name": "SOC 2 — Logical Access (CC6)",
  "type": "attestation",
  "criteria_source": "AICPA Trust Services Criteria …",
  "disclaimer": "Aurelion provides evidence usable in a SOC 2 examination; it does not certify compliance.",
  "controls": [
    {
      "control_id": "CC6.1",
      "title": "Logical access security …",
      "criteria": "…",
      "category": "logical-access",
      "violating_kinds": ["mfa_less_privileged_access", "privileged_access", "…"],
      "population": ["account", "secret_plain", "secret_certificate"]
    }
  ]
}
```

The engine discovers definitions by listing cartridges through the
platform provider and reading the `projection.json` of each
materialised cartridge — it never decides how a cartridge is fetched.
Enabling another projection is dropping another cartridge; the engine is
untouched.

`population` is descriptive scope metadata returned in the control
detail; the coverage computation does not read it (violations, gaps, and
the evaluated signal come from finding kinds and the rule→kind map).

`type` names the external-language shelf — these are not interchangeable
artifacts: `attestation` (an auditor's report over a period, e.g. SOC 2)
is a different kind of thing from a `control_catalog` (NIST 800-53),
`certifiable_standard` (ISO 27001), or `prescriptive_baseline` (CIS).
The engine treats every projection uniformly; consumers keep the shelves
distinct.

## HTTP surface (read-only)

Mounted under the assessment-run path:

| Method | Path | Description |
|---|---|---|
| GET | `/policy-assessment-runs/:id/projections` | Projections available for the run + coverage roll-up |
| GET | `/policy-assessment-runs/:id/projections/:projection` | Per-control coverage table |
| GET | `/policy-assessment-runs/:id/projections/:projection/controls/:controlID` | One control: state + supporting findings + blind spots |
| GET | `/policy-assessment-runs/:id/projections/:projection/packet` | Evidence packet (JSON; `?format=csv` for working papers) |

## Dependencies

Reads only — through narrow ports (`CartridgeReader`, `FindingsReader`,
`OutcomesReader`, `RunReader`). Downward deps to inventory L1
(`findings`, `policy_assessment_runs`, `policy_evaluation_outcomes`) and
the platform cartridges provider. No event emission, no writes.

## Not in this package

- **Policies** — projections never evaluate; they map the output of
  policies that already ran (`policy_assessment`).
- **Persistence / history** — coverage is recomputed per request.
- **Period-semantics evaluation** — the projection states the run's time
  window (`period`) but does not itself run period-mode evaluation; that
  is the assessment run's concern.
- **Certification claims** — the engine emits a `disclaimer`; it never
  asserts an audit opinion.
