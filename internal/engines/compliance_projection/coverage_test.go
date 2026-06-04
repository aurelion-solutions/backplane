// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package compliance_projection

import (
	"testing"

	peo "github.com/aurelion-solutions/backplane/internal/inventory/policy_evaluation_outcomes"
)

func outcome(ruleID, result string) *peo.PolicyEvaluationOutcome {
	return &peo.PolicyEvaluationOutcome{RuleID: ruleID, Outcome: result}
}

func TestComputeControl_States(t *testing.T) {
	c := Control{ControlID: "CC6.1", ViolatingKinds: []string{"privileged_access", "weak_certificate_key"}}
	// rule r1 emits privileged_access, r2 emits weak_certificate_key.
	ruleKind := map[string]string{"r1": "privileged_access", "r2": "weak_certificate_key"}

	cases := []struct {
		name      string
		findings  map[string]int
		outcomes  []*peo.PolicyEvaluationOutcome
		wantState string
		wantViol  int
		wantGaps  int
	}{
		{
			name:      "covered — evaluated, no violations, no gaps",
			findings:  map[string]int{},
			outcomes:  []*peo.PolicyEvaluationOutcome{outcome("r1", peo.OutcomeNotMatched), outcome("r2", peo.OutcomeNotMatched)},
			wantState: StateCovered,
		},
		{
			name:      "failed — a violation, no gaps",
			findings:  map[string]int{"privileged_access": 3},
			outcomes:  []*peo.PolicyEvaluationOutcome{outcome("r1", peo.OutcomeMatched)},
			wantState: StateFailed,
			wantViol:  3,
		},
		{
			name:      "partial — violation AND gap",
			findings:  map[string]int{"privileged_access": 1},
			outcomes:  []*peo.PolicyEvaluationOutcome{outcome("r1", peo.OutcomeMatched), outcome("r2", peo.OutcomeNotEvaluable)},
			wantState: StatePartial,
			wantViol:  1,
			wantGaps:  1,
		},
		{
			name:      "not_evaluable — gap, no violation",
			findings:  map[string]int{},
			outcomes:  []*peo.PolicyEvaluationOutcome{outcome("r1", peo.OutcomeNotEvaluable)},
			wantState: StateNotEvaluable,
			wantGaps:  1,
		},
		{
			name:      "not_evaluable — population never reached (no PEO for any control rule)",
			findings:  map[string]int{},
			outcomes:  []*peo.PolicyEvaluationOutcome{outcome("unrelated", peo.OutcomeMatched)},
			wantState: StateNotEvaluable,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := computeControl(c, gatherControlInputs(c, tc.findings, tc.outcomes, ruleKind))
			if got.State != tc.wantState {
				t.Errorf("state = %q, want %q", got.State, tc.wantState)
			}
			if got.Violations != tc.wantViol {
				t.Errorf("violations = %d, want %d", got.Violations, tc.wantViol)
			}
			if got.Gaps != tc.wantGaps {
				t.Errorf("gaps = %d, want %d", got.Gaps, tc.wantGaps)
			}
		})
	}
}

func TestComputeCoverage_Summary(t *testing.T) {
	def := Definition{
		Projection: "soc2-logical-access",
		Name:       "SOC 2",
		Controls: []Control{
			{ControlID: "CC6.1", ViolatingKinds: []string{"privileged_access"}},
			{ControlID: "CC6.2", ViolatingKinds: []string{"orphan_access"}},
		},
	}
	ruleKind := map[string]string{"r1": "privileged_access", "r2": "orphan_access"}
	findingsByKind := map[string]int{"privileged_access": 2}
	outcomes := []*peo.PolicyEvaluationOutcome{
		outcome("r1", peo.OutcomeMatched),    // CC6.1 → failed
		outcome("r2", peo.OutcomeNotMatched), // CC6.2 → covered
	}

	controls, summary := computeCoverage(def, findingsByKind, outcomes, ruleKind)
	if len(controls) != 2 {
		t.Fatalf("controls = %d, want 2", len(controls))
	}
	if summary[StateFailed] != 1 || summary[StateCovered] != 1 {
		t.Errorf("summary = %+v, want 1 failed + 1 covered", summary)
	}
}
