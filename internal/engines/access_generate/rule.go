// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package access_generate

import (
	"encoding/json"
	"errors"
	"fmt"
)

// MechanismInheritance is the value of `Manifest.Mechanism` that
// flags a policy as an inheritance rule for this engine. Policies
// with any other mechanism string are ignored by the inheritance
// resolver.
const MechanismInheritance = "inheritance"

// InheritanceRule is the parsed body of one `.meta.json` whose
// mechanism is `inheritance`.
//
// Shape inside `Manifest.Body`:
//
//	{
//	  "source_org_unit_dn": "corp/europe/engineering",
//	  "grants": [
//	    { "application_slug": "microsoft_ad" },
//	    { "application_slug": "github", "capability_slug": "developer" }
//	  ]
//	}
//
// `application_slug` resolves to an `applications.id` at evaluation
// time; `capability_slug` to a `capabilities.id`. A grant without
// `capability_slug` is an account-initiative; a grant with one is a
// grant-initiative on that capability.
type InheritanceRule struct {
	RuleID          string
	SourceOrgUnitDN string
	Grants          []RuleGrant
}

// RuleGrant is one entry from `grants:[]` inside an inheritance
// rule body.
type RuleGrant struct {
	ApplicationSlug string
	CapabilitySlug  string // empty → account-initiative
}

// inheritanceBody mirrors the JSON shape inside Manifest.Body. Kept
// private — callers always go through ParseInheritanceRule.
type inheritanceBody struct {
	SourceOrgUnitDN string `json:"source_org_unit_dn"`
	Grants          []struct {
		ApplicationSlug string `json:"application_slug"`
		CapabilitySlug  string `json:"capability_slug"`
	} `json:"grants"`
}

// ParseInheritanceRule turns a Manifest body into an InheritanceRule.
// Returns an error when required fields are missing or empty, so a
// malformed rule does not silently disappear from the projection.
func ParseInheritanceRule(ruleID string, rawBody map[string]any) (*InheritanceRule, error) {
	if len(rawBody) == 0 {
		return nil, fmt.Errorf("rule %q: empty body", ruleID)
	}
	raw, err := json.Marshal(rawBody)
	if err != nil {
		return nil, fmt.Errorf("rule %q: marshal body: %w", ruleID, err)
	}
	var b inheritanceBody
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("rule %q: parse body: %w", ruleID, err)
	}
	if b.SourceOrgUnitDN == "" {
		return nil, fmt.Errorf("rule %q: source_org_unit_dn is required", ruleID)
	}
	if len(b.Grants) == 0 {
		return nil, errors.New("rule " + ruleID + ": grants must be non-empty")
	}
	out := &InheritanceRule{
		RuleID:          ruleID,
		SourceOrgUnitDN: b.SourceOrgUnitDN,
		Grants:          make([]RuleGrant, 0, len(b.Grants)),
	}
	for i, g := range b.Grants {
		if g.ApplicationSlug == "" {
			return nil, fmt.Errorf("rule %q: grants[%d].application_slug is required", ruleID, i)
		}
		out.Grants = append(out.Grants, RuleGrant{
			ApplicationSlug: g.ApplicationSlug,
			CapabilitySlug:  g.CapabilitySlug,
		})
	}
	return out, nil
}
