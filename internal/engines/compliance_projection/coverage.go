// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package compliance_projection

import (
	"github.com/aurelion-solutions/backplane/internal/inventory/policy_evaluation_outcomes"
)

// controlInputs is the pre-aggregated evidence one control is scored
// against: how many baseline findings of its violating kinds are
// present, whether any rule it depends on was evaluated at all, and how
// many of those rules came back not_evaluable (gaps).
type controlInputs struct {
	violations int
	gaps       int
	evaluated  bool
}

// computeControl rolls one control's evidence into a coverage state.
//
// The state machine is deliberately conservative about `covered`: it is
// reached only on positive evidence — the population was evaluated and
// produced neither a violation nor a gap. A population no rule reached,
// or one with an unresolved gap, is `not_evaluable`, never covered.
func computeControl(c Control, in controlInputs) ControlCoverage {
	state := StateCovered
	switch {
	case !in.evaluated:
		state = StateNotEvaluable
	case in.violations > 0 && in.gaps > 0:
		state = StatePartial
	case in.violations > 0:
		state = StateFailed
	case in.gaps > 0:
		state = StateNotEvaluable
	}
	return ControlCoverage{
		ControlID:  c.ControlID,
		Title:      c.Title,
		Category:   c.Category,
		State:      state,
		Violations: in.violations,
		Gaps:       in.gaps,
		Evaluated:  in.evaluated,
	}
}

// gatherControlInputs aggregates the evidence for one control from the
// run's findings-by-kind tally, its policy-evaluation outcomes, and the
// rule→kind map derived from cartridge manifests.
//
//   - violations: baseline findings whose kind the control flags as
//     violating.
//   - evaluated: at least one rule emitting one of the control's
//     violating kinds produced ANY outcome this run (the population was
//     reached).
//   - gaps: those rules that came back not_evaluable (reached but blind).
func gatherControlInputs(
	c Control,
	findingsByKind map[string]int,
	outcomes []*policy_evaluation_outcomes.PolicyEvaluationOutcome,
	ruleKind map[string]string,
) controlInputs {
	violating := make(map[string]struct{}, len(c.ViolatingKinds))
	for _, k := range c.ViolatingKinds {
		violating[k] = struct{}{}
	}

	in := controlInputs{}
	for k := range violating {
		in.violations += findingsByKind[k]
	}
	for _, o := range outcomes {
		kind, ok := ruleKind[o.RuleID]
		if !ok {
			continue
		}
		if _, relevant := violating[kind]; !relevant {
			continue
		}
		in.evaluated = true
		if o.Outcome == policy_evaluation_outcomes.OutcomeNotEvaluable {
			in.gaps++
		}
	}
	return in
}

// computeCoverage rolls every control in a definition up into a coverage
// report body (controls + summary). It is pure over its inputs — the
// service fetches findings/outcomes/period and calls this.
func computeCoverage(
	def Definition,
	findingsByKind map[string]int,
	outcomes []*policy_evaluation_outcomes.PolicyEvaluationOutcome,
	ruleKind map[string]string,
) ([]ControlCoverage, map[string]int) {
	controls := make([]ControlCoverage, 0, len(def.Controls))
	summary := map[string]int{
		StateCovered:      0,
		StateFailed:       0,
		StatePartial:      0,
		StateNotEvaluable: 0,
	}
	for _, c := range def.Controls {
		cc := computeControl(c, gatherControlInputs(c, findingsByKind, outcomes, ruleKind))
		controls = append(controls, cc)
		summary[cc.State]++
	}
	return controls, summary
}
