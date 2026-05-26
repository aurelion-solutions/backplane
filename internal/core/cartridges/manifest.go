// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package cartridges

import (
	"encoding/json"
	"fmt"
	"os"
)

// Manifest is the in-memory projection of one <rule>.meta.json sidecar
// inside a cartridge. Mechanism is a plain string at this layer — the
// platform doesn't know domain enums; consumer engines validate it
// against their own mechanism handler registry (cedar, sod,
// risk_scoring, llm_classification, …).
//
// The Body field carries the open-ended payload that's
// mechanism-specific: weights / thresholds for risk_scoring,
// prompt_template_file for llm_classification, policy_file for cedar,
// etc. The platform doesn't interpret it.
type Manifest struct {
	RuleID      string `json:"rule_id"`
	Version     int    `json:"version"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Mechanism   string `json:"mechanism"`
	Severity    string `json:"severity,omitempty"`
	OwnerTeam   string `json:"owner_team,omitempty"`
	// Tags are facets used by the runtime to coarse-filter applicable
	// policies before calling the mechanism handler. Typical entries:
	// "authn", "authz", "transport:saml", "geo:eu", "scan",
	// "framework:sox". Format is free — runtime treats them as opaque
	// strings; matching is "every tag in policy.tags must appear in
	// request.facets" (subset containment).
	Tags []string `json:"tags,omitempty"`
	// Body is the open-ended payload mechanism handlers consume —
	// policy_file for cedar, prompt_file + response_schema for
	// llm_classification, weights + thresholds for risk_scoring, etc.
	// The platform layer does not interpret it.
	Body map[string]any `json:"body,omitempty"`
	// StackCheck is the optional truth-stack precondition. When present,
	// it declares the truth-input keys this policy needs; the
	// policy-assessment engine marks the evaluation not_evaluable when
	// any required key is absent, rather than running the mechanism on
	// incomplete evidence. Absent = no precondition (binary path).
	StackCheck *StackCheck `json:"stack_check,omitempty"`
	// Finding carries the cartridge-authored finding metadata: title,
	// severity, and the operator-facing remediation guidance. Engines
	// denormalise the remediation onto emitted findings so the
	// assessment UI can show a suggested action sourced from the
	// cartridge, not a separate remediation capability.
	Finding *FindingMeta `json:"finding,omitempty"`
	// DefaultRecommendation is the cartridge's suggested action verb for
	// this finding (e.g. "review", "revoke", "owner_assign",
	// "policy_fix").
	DefaultRecommendation string `json:"default_recommendation,omitempty"`
	// BasePath is the absolute path of this manifest's .meta.json file.
	// Populated by the cartridges Provider after the sidecar is read;
	// not a user-authored field. Mechanism handlers use it as the
	// anchor to resolve their own sibling files (e.g. .cedar,
	// .prompt, .yaml).
	BasePath string `json:"-"`
}

// StackCheck declares the truth-stack inputs a policy requires to be
// evaluable. Requires lists truth-input keys (e.g. "mfa_evidence");
// the engine marks the evaluation not_evaluable when any is absent.
type StackCheck struct {
	Requires []string `json:"requires,omitempty"`
}

// FindingMeta is the cartridge-authored description of the finding a
// policy emits — surfaced to operators, not interpreted by the platform.
type FindingMeta struct {
	Kind        string `json:"kind,omitempty"`
	Title       string `json:"title,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Remediation string `json:"remediation,omitempty"`
}

// loadManifest reads one .meta.json sidecar and validates the minimum
// required fields. RuleID, Version, Name, Mechanism are mandatory.
func loadManifest(path string) (Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: read %q: %v", ErrInvalidManifest, path, err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return Manifest{}, fmt.Errorf("%w: parse %q: %v", ErrInvalidManifest, path, err)
	}
	if m.RuleID == "" {
		return Manifest{}, fmt.Errorf("%w: %q: rule_id is required", ErrInvalidManifest, path)
	}
	if m.Version == 0 {
		return Manifest{}, fmt.Errorf("%w: %q: version is required", ErrInvalidManifest, path)
	}
	if m.Name == "" {
		return Manifest{}, fmt.Errorf("%w: %q: name is required", ErrInvalidManifest, path)
	}
	if m.Mechanism == "" {
		return Manifest{}, fmt.Errorf("%w: %q: mechanism is required", ErrInvalidManifest, path)
	}
	return m, nil
}
