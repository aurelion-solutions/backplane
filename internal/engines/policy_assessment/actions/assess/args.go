// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package assess

// Args is the input contract for the policy_assessment.assess action.
//
// Scope narrows the population: ApplicationID restricts to one
// application's accounts. Mechanisms restricts to the named handler
// allowlist — empty means "every mechanism registered in this
// process". TriggeredBy lands on the assessment-run row as audit
// metadata.
type Args struct {
	TriggeredBy   string   `json:"triggered_by,omitempty"`
	ApplicationID string   `json:"application_id,omitempty"`
	Mechanisms    []string `json:"mechanisms,omitempty"`
	CreatedBy     string   `json:"created_by,omitempty"`
}

// Result is the output contract — counters for observability.
type Result struct {
	AssessmentRunID   string `json:"assessment_run_id"`
	AccountsEvaluated int    `json:"accounts_evaluated"`
	PoliciesApplied   int    `json:"policies_applied"`
	Matched           int    `json:"matched"`
	FindingsCreated   int    `json:"findings_created"`
	FindingsReused    int    `json:"findings_reused"`
}
