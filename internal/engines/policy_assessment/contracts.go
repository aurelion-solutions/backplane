// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policy_assessment

// Request is the input to one mechanism evaluation. Built by the caller
// (PDP transport / policy-assessment action) from its own envelope and handed to
// the dispatcher, which routes by Mechanism.
//
// Body is the policy's mechanism-specific payload (from
// cartridges.Manifest.Body) — policy_file for cedar,
// prompt_template_file for llm_classification, etc. The dispatcher does
// not interpret it; the mechanism handler does.
//
// Facts is the canonical PDP input shared across every mechanism — see
// schemas.go and the kernel RULE_CONTRACT.md.
type Request struct {
	// Mechanism names the handler — "cedar", "sod",
	// "llm_classification", …
	Mechanism string

	// PolicyID is the cartridge-scoped rule id ("<cartridge>/<rule_id>"),
	// surfaced on findings and decision reasons.
	PolicyID string

	// CartridgeRef is the namespace the policy comes from.
	CartridgeRef string

	// BasePath is the absolute path of the policy's .meta.json file;
	// handlers resolve their sibling artifacts (.cedar, .prompt) from
	// it.
	BasePath string

	// Body is the mechanism-specific payload from the manifest.
	Body map[string]any

	// Facts is the runtime input — subject / target / action / context
	// / threat / now. Mechanism handlers read only the sections they
	// care about; the platform supplies whatever the caller gathered.
	Facts Facts
}

// Output is the dispatcher's return value — one RuleResult plus
// per-call diagnostics. Aggregation rules live at the caller (deny-wins
// for AuthZ, append-all for scan, score-max for risk_scoring).
type Output struct {
	// Matched is true when the mechanism considers the policy
	// applicable to the request — its rule body fired or its
	// generative path produced facts. A non-matching policy
	// contributes nothing to aggregation.
	Matched bool

	// Result carries the canonical RuleResult (Decision and/or
	// ProjectedFacts). May be empty when the rule did not fire.
	Result RuleResult

	// Signals are discrete markers the policy emitted at the
	// dispatcher level (mechanism telemetry, not rule signals — those
	// live inside Result.Decision.Signals as raw any/dict).
	Signals []AssessmentSignal

	// Evidence is supporting data — references / excerpts / values
	// that informed the decision. Surfaces in audit and finding
	// payloads.
	Evidence []Evidence

	// Explanation is an optional human-readable summary.
	Explanation string

	// Confidence is in [0, 1] when the mechanism can estimate it,
	// nil otherwise.
	Confidence *float64

	// Payload carries mechanism-specific extras for debugging /
	// tracing; not part of the stable contract.
	Payload map[string]any
}

// AssessmentSignal is a discrete marker emitted at the engine level
// (dispatcher / mechanism telemetry — not a rule-level signal).
//
// Rule-level signals live inside Decision.Signals / ProjectedFact.Signals
// as polymorphic `any` entries (plain string code or structured dict),
// mirroring the kernel `Signal = str | dict` union. This struct is the
// normalized engine-side companion used by Output.Signals — when the
// mechanism wants to surface a typed marker with severity/message
// alongside the raw rule output.
type AssessmentSignal struct {
	Code     string
	Severity string
	Message  string
	Payload  map[string]any
}

// Evidence is the supporting data point that informed the assessment.
type Evidence struct {
	SourceType string
	SourceID   string
	Title      string
	Summary    string
	Payload    map[string]any
}
