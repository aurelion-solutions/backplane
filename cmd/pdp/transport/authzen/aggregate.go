// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package authzen

import (
	"github.com/aurelion-solutions/backplane/internal/engines/policy_assessment"
)

// perPolicyOutput is one policy's evaluation result + the identity of
// the policy that produced it.
type perPolicyOutput struct {
	PolicyID  string
	Cartridge string
	Output    policy_assessment.Output
	Err       error
}

// aggregate folds N per-policy outputs into one AuthZen response.
//
// Rules:
//
//   - Any Decision.Effect == "deny" → response.decision = false.
//   - Otherwise ≥1 Decision.Effect == "allow" → response.decision = true.
//   - Neither → default deny.
//
// Reasons list includes every applicable policy with its effect; the
// first kernel Reason's RuleID (typically the cartridge policy id)
// lands in ReasonItem.Reason for quick eyeball. AuthZen obligations
// are not part of the kernel rule contract — Response.Context.Obligations
// stays empty here; if a transport wants to surface them, it does so by
// extracting structured signals from Decision.Signals (which is
// polymorphic `[]any` and can carry `{"kind": "obligation", ...}`
// dicts).
func aggregate(outs []perPolicyOutput) Response {
	resp := Response{
		Decision: false,
		Context:  RespContext{},
	}
	resp.Context.RulesCount = len(outs)

	anyAllow := false
	anyDeny := false
	for _, p := range outs {
		if p.Err != nil {
			resp.Context.Reasons = append(resp.Context.Reasons, ReasonItem{
				PolicyID:  p.PolicyID,
				Cartridge: p.Cartridge,
				Reason:    "evaluator_error: " + p.Err.Error(),
			})
			continue
		}
		dec := p.Output.Result.Decision
		if !p.Output.Matched && dec == nil {
			continue
		}
		eff := ""
		reason := ""
		if dec != nil {
			eff = dec.Effect
			if len(dec.Reasons) > 0 {
				reason = dec.Reasons[0].RuleID
			}
		}
		switch eff {
		case "allow":
			anyAllow = true
		case "deny":
			anyDeny = true
		}
		resp.Context.Reasons = append(resp.Context.Reasons, ReasonItem{
			PolicyID:  p.PolicyID,
			Cartridge: p.Cartridge,
			Effect:    eff,
			Reason:    reason,
		})
	}
	switch {
	case anyDeny:
		resp.Decision = false
	case anyAllow:
		resp.Decision = true
	default:
		resp.Decision = false
	}
	return resp
}
