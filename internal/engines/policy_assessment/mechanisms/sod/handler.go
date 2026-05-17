// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package sod is the Segregation-of-Duties mechanism handler.
//
// Detects toxic combinations of capabilities held by a single
// principal: "creates POs and approves POs", "modifies vendor master
// and processes payments", and similar pairs / triples enumerated by
// the rule's conditions.
//
// Rule body shape (parsed from Manifest.Body):
//
//	{
//	  "conditions": [
//	    {"capability_slugs": ["payments.creator"], "min_count": 1},
//	    {"capability_slugs": ["payments.approver"], "min_count": 1}
//	  ]
//	}
//
// Evaluation is pure: handler reads
// `req.Facts.Principal.CapabilitySlugs`, intersects each condition's
// `capability_slugs` against that set, and checks `min_count`. When
// every condition is satisfied the rule fires and emits a Decision
// with `risk_level` (from manifest severity, default "high"),
// polymorphic signals (a string code plus a structured payload listing
// the matched capabilities per condition), and a reasons block.
//
// scope_mode (`global` / `per_application` / `by_scope_key`) is NOT
// honoured in this revision — the engine input does not carry
// per-grant scope context yet. Conditions evaluate over the full
// CapabilitySlugs set as if `scope_mode: global`.
package sod

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/aurelion-solutions/backplane/internal/engines/policy_assessment"
)

// Mechanism is the policy_assessment registry key.
const Mechanism = "sod"

// Handler is the SoD mechanism handler. One instance serves every
// SoD policy in the catalogue; per-policy state (parsed conditions)
// is cached internally.
type Handler struct {
	mu     sync.RWMutex
	caches map[string]*ruleCache // keyed by CartridgeRef + "/" + RuleID
}

// ruleCache holds one parsed rule body.
type ruleCache struct {
	conditions []condition
}

// condition is one min-count clause from manifest body.
type condition struct {
	Slugs    []string `json:"capability_slugs"`
	MinCount int      `json:"min_count"`
}

// New returns an empty Handler.
func New() *Handler {
	return &Handler{caches: map[string]*ruleCache{}}
}

// Mechanism returns the registry key.
func (h *Handler) Mechanism() string { return Mechanism }

// Prepare parses the manifest body, validates it, and caches the
// parsed conditions. A body with no conditions, or a condition with
// no capability_slugs / min_count <= 0, is rejected — the rule could
// never fire and would just clutter the catalogue.
func (h *Handler) Prepare(_ context.Context, entry policy_assessment.Entry) error {
	conds, err := parseConditions(entry.Manifest.Body)
	if err != nil {
		return fmt.Errorf("sod: %s/%s: %w", entry.CartridgeRef, entry.Manifest.RuleID, err)
	}
	key := cacheKey(entry)
	h.mu.Lock()
	h.caches[key] = &ruleCache{conditions: conds}
	h.mu.Unlock()
	return nil
}

// Evaluate runs the prepared SoD rule against the principal's
// CapabilitySlugs. Returns Matched=true only when every condition's
// MinCount of slugs is covered.
func (h *Handler) Evaluate(_ context.Context, req policy_assessment.Request) (policy_assessment.Output, error) {
	h.mu.RLock()
	cache, ok := h.caches[reqKey(req)]
	h.mu.RUnlock()
	if !ok {
		return policy_assessment.Output{}, fmt.Errorf("sod: policy not prepared: %s", req.PolicyID)
	}
	if req.Facts.Principal == nil {
		return policy_assessment.Output{Matched: false}, nil
	}
	held := slugSet(req.Facts.Principal.CapabilitySlugs)
	if len(held) == 0 {
		return policy_assessment.Output{Matched: false}, nil
	}

	matchedPerCondition := make([][]string, 0, len(cache.conditions))
	for _, c := range cache.conditions {
		matched := intersect(held, c.Slugs)
		if len(matched) < c.MinCount {
			// Any condition shortfall → rule does not fire.
			return policy_assessment.Output{Matched: false}, nil
		}
		matchedPerCondition = append(matchedPerCondition, matched)
	}

	// Severity lands on the finding row from the manifest static
	// field via the action. The Decision carries risk_level as a
	// dynamic signal — SoD conflicts default to high; future
	// revisions may scale by privilege level of matched capabilities.
	risk := policy_assessment.RiskHigh

	conflict := map[string]any{
		"kind":       "sod_conflict",
		"principal":  req.Facts.Principal.ID,
		"conditions": conflictConditions(cache.conditions, matchedPerCondition),
	}
	if req.Facts.Principal.Kind != "" {
		conflict["principal_kind"] = req.Facts.Principal.Kind
	}

	out := policy_assessment.Output{
		Matched: true,
		Result: policy_assessment.RuleResult{
			Decision: &policy_assessment.Decision{
				RiskLevel: risk,
				Signals:   []any{"sod_conflict", conflict},
				Reasons: []policy_assessment.Reason{
					{
						RuleID:   req.PolicyID,
						RuleKind: "anomaly",
						MatchedConditions: map[string]any{
							"conditions_met": len(cache.conditions),
						},
						FactValues: map[string]any{
							"principal.id":              req.Facts.Principal.ID,
							"principal.capability_slugs": req.Facts.Principal.CapabilitySlugs,
						},
						Produced: map[string]any{
							"matched_per_condition": matchedPerCondition,
						},
					},
				},
			},
		},
	}
	return out, nil
}

// --- helpers --------------------------------------------------------

// parseConditions extracts the conditions block from manifest body.
// JSON-roundtrips through a typed slice so we get unmarshal validation
// without writing a hand-rolled parser.
func parseConditions(body map[string]any) ([]condition, error) {
	raw, ok := body["conditions"]
	if !ok {
		return nil, errors.New("body.conditions missing")
	}
	bytes, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshal conditions: %w", err)
	}
	var conds []condition
	if err := json.Unmarshal(bytes, &conds); err != nil {
		return nil, fmt.Errorf("unmarshal conditions: %w", err)
	}
	if len(conds) == 0 {
		return nil, errors.New("body.conditions is empty")
	}
	for i, c := range conds {
		if len(c.Slugs) == 0 {
			return nil, fmt.Errorf("condition[%d].capability_slugs is empty", i)
		}
		if c.MinCount <= 0 {
			return nil, fmt.Errorf("condition[%d].min_count must be > 0", i)
		}
	}
	return conds, nil
}

// slugSet returns the principal's capability slugs as a set.
func slugSet(slugs []string) map[string]struct{} {
	out := make(map[string]struct{}, len(slugs))
	for _, s := range slugs {
		out[s] = struct{}{}
	}
	return out
}

// intersect returns the subset of `wanted` that the principal holds,
// preserving the original order of `wanted` for deterministic output.
func intersect(held map[string]struct{}, wanted []string) []string {
	matched := make([]string, 0, len(wanted))
	for _, w := range wanted {
		if _, ok := held[w]; ok {
			matched = append(matched, w)
		}
	}
	return matched
}

// conflictConditions builds the audit-friendly per-condition record
// for the structured Signal payload.
func conflictConditions(conds []condition, matched [][]string) []any {
	out := make([]any, len(conds))
	for i, c := range conds {
		out[i] = map[string]any{
			"required":  c.Slugs,
			"min_count": c.MinCount,
			"matched":   matched[i],
		}
	}
	return out
}

func cacheKey(entry policy_assessment.Entry) string {
	return entry.CartridgeRef + "/" + entry.Manifest.RuleID
}

func reqKey(req policy_assessment.Request) string {
	return req.CartridgeRef + "/" + ruleIDFromPolicyID(req.PolicyID, req.CartridgeRef)
}

func ruleIDFromPolicyID(policyID, cartridge string) string {
	prefix := cartridge + "/"
	if len(policyID) > len(prefix) && policyID[:len(prefix)] == prefix {
		return policyID[len(prefix):]
	}
	return policyID
}
