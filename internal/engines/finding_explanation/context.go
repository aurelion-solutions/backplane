// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package finding_explanation

import (
	"fmt"
	"strings"

	"github.com/aurelion-solutions/backplane/internal/inventory/evidence_chain"
	"github.com/aurelion-solutions/backplane/internal/inventory/findings"
)

// explanationContext is the assembled, labelled fact set handed to the
// model. Refs is the validation boundary: a citation in the narrative
// counts only if its label appears here.
type explanationContext struct {
	finding *findings.Finding
	refs    []Reference
}

// collectContext labels a finding, its policy/source and its evidence
// chain into the reference set the model is allowed to cite. The labels
// are stable and ordered: F (finding), P1 (policy), S (source), then
// E1..En (evidence chains in chain order).
func collectContext(f *findings.Finding, chains []*evidence_chain.EvidenceChain) explanationContext {
	refs := make([]Reference, 0, len(chains)+3)

	refs = append(refs, Reference{
		Label:  "F",
		Kind:   RefFinding,
		ID:     f.ID.String(),
		Detail: findingDetail(f),
	})

	if f.PolicyID != nil {
		detail := "policy " + f.PolicyID.String()
		if f.CartridgeRef != nil {
			detail = fmt.Sprintf("policy %s (cartridge %s)", f.PolicyID.String(), *f.CartridgeRef)
		}
		refs = append(refs, Reference{
			Label:  "P1",
			Kind:   RefPolicy,
			ID:     f.PolicyID.String(),
			Detail: detail,
		})
	}

	if f.Source != nil && *f.Source != "" {
		id := *f.Source
		if f.ApplicationID != nil {
			id = f.ApplicationID.String()
		}
		refs = append(refs, Reference{
			Label:  "S",
			Kind:   RefSource,
			ID:     id,
			Detail: "source system " + *f.Source,
		})
	}

	for i, c := range chains {
		refs = append(refs, Reference{
			Label:  fmt.Sprintf("E%d", i+1),
			Kind:   RefEvidence,
			ID:     c.ID.String(),
			Detail: evidenceDetail(c),
		})
	}

	return explanationContext{finding: f, refs: refs}
}

// findingDetail renders the finding's deterministic facts into one line.
func findingDetail(f *findings.Finding) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s severity=%s status=%s", f.Kind, f.Severity, f.Status)
	if f.TargetType != nil {
		fmt.Fprintf(&b, " target=%s", *f.TargetType)
		if f.TargetID != nil {
			fmt.Fprintf(&b, ":%s", f.TargetID.String())
		}
	}
	if f.PrincipalID != nil {
		fmt.Fprintf(&b, " principal=%s", f.PrincipalID.String())
	}
	if f.ScopeValue != nil && *f.ScopeValue != "" {
		fmt.Fprintf(&b, " scope=%s", *f.ScopeValue)
	}
	return b.String()
}

// evidenceDetail renders one evidence chain's anchoring facts.
func evidenceDetail(c *evidence_chain.EvidenceChain) string {
	var b strings.Builder
	fmt.Fprintf(&b, "policy_ref=%s", c.PolicyRef)
	if c.NormalizedKind != nil {
		fmt.Fprintf(&b, " normalized=%s", *c.NormalizedKind)
		if c.NormalizedID != nil {
			fmt.Fprintf(&b, ":%s", c.NormalizedID.String())
		}
	}
	fmt.Fprintf(&b, " chain_hash=%s", c.ChainHash)
	return b.String()
}
