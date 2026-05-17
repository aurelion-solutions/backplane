// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package authzen

// Request mirrors the OpenID AuthZen 1.0 Access Evaluation request
// envelope.
type Request struct {
	Subject  Subject        `json:"subject"`
	Resource Resource       `json:"resource"`
	Action   Action         `json:"action"`
	Context  map[string]any `json:"context,omitempty"`
}

// Subject is the identity the caller wants to evaluate against.
type Subject struct {
	Type       string         `json:"type"`
	ID         string         `json:"id"`
	Properties map[string]any `json:"properties,omitempty"`
}

// Resource is the object of the access decision.
type Resource struct {
	Type       string         `json:"type"`
	ID         string         `json:"id"`
	Properties map[string]any `json:"properties,omitempty"`
}

// Action is the verb being evaluated.
type Action struct {
	Name       string         `json:"name"`
	Properties map[string]any `json:"properties,omitempty"`
}

// Response is the PDP's reply to one Request.
type Response struct {
	Decision bool        `json:"decision"`
	Context  RespContext `json:"context"`
}

// RespContext carries the per-policy diagnostics + obligations the
// caller acts on.
type RespContext struct {
	Reasons     []ReasonItem     `json:"reasons,omitempty"`
	Obligations []map[string]any `json:"obligations,omitempty"`
	RulesCount  int              `json:"rules_count"`
}

// ReasonItem is one row in the response reasons array.
type ReasonItem struct {
	PolicyID  string `json:"policy_id"`
	Cartridge string `json:"cartridge"`
	Effect    string `json:"effect,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// Validate runs the cheap structural checks before dispatching.
func (r Request) Validate() error {
	if r.Subject.Type == "" || r.Subject.ID == "" {
		return errSubjectMissing
	}
	if r.Resource.Type == "" {
		return errResourceTypeMissing
	}
	if r.Action.Name == "" {
		return errActionMissing
	}
	return nil
}
