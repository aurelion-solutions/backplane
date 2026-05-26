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
	// Cartridges restricts evaluation to the selected cartridge refs
	// (the campaign's selected content). Empty = every loaded cartridge.
	Cartridges []string `json:"cartridges,omitempty"`
	// AsOf is the campaign's point-in-time lake timestamp (RFC3339).
	// Stamped onto the run; the lake is current-only today, so this is
	// recorded for provenance until historical queries land (Slice 5).
	AsOf string `json:"as_of,omitempty"`
}

// Result is the output contract — counters for observability.
type Result struct {
	AssessmentRunID    string `json:"assessment_run_id"`
	AccountsEvaluated  int    `json:"accounts_evaluated"`
	WorkloadsEvaluated int    `json:"workloads_evaluated"`
	SecretsEvaluated   int    `json:"secrets_evaluated"`
	ConsentEvaluated   int    `json:"consent_evaluated"`
	PoliciesApplied    int    `json:"policies_applied"`
	Matched            int    `json:"matched"`
	NotMatched         int    `json:"not_matched"`
	NotEvaluable       int    `json:"not_evaluable"`
	FindingsCreated    int    `json:"findings_created"`
	FindingsReused     int    `json:"findings_reused"`
	EvidenceGaps       int    `json:"evidence_gaps"`
	ChainsRecorded     int    `json:"chains_recorded"`
}
