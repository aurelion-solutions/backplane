// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package policy_evaluation_outcomes (PEO) persists the ternary result
// of every (policy, target) evaluation in an assessment run.
//
// findings records only matched violations. PEO is the superset that
// also records not_matched (clean) and not_evaluable (Blind Spots —
// the rule could not be evaluated because a required truth input is
// absent). not_evaluable is a first-class product output, not an error.
//
// Invariant (the biconditional): outcome = not_evaluable if and only if
// missing_evidence is non-empty. RecordOutcome enforces it; the DB
// CHECK constraints pin the closed sets for outcome and target_type.
//
// Identity is (assessment_run_id, cartridge_id, rule_id, target_type,
// target_ref); re-emission within the same run upserts rather than
// duplicating. Deriving an evidence_gap Finding from a not_evaluable
// outcome is the orchestrator's job (the policy-assessment action),
// not this slice — PEO stays free of a sibling findings dependency.
package policy_evaluation_outcomes
