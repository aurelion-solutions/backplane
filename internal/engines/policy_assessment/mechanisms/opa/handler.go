// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package opa is the Rego predicate-evaluator mechanism for the
// policy_assessment engine. Backend evaluator is the embedded OPA
// `rego.PreparedEvalQuery` — in-process eval, no sidecar.
//
// One handler instance serves every Rego policy in the catalogue;
// per-policy state (parsed package path + prepared query) is cached
// internally and refreshed on Prepare.
package opa

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aurelion-solutions/backplane/internal/engines/policy_assessment"
	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/rego"
)

func parseTimeRFC3339(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

// Mechanism is the policy_assessment registry key.
const Mechanism = "opa"

// Handler is the OPA mechanism handler.
type Handler struct {
	mu     sync.RWMutex
	caches map[string]*policyCache // keyed by CartridgeRef + "/" + RuleID
}

// policyCache holds the prepared eval query plus the parsed package
// path the query targets.
type policyCache struct {
	query   rego.PreparedEvalQuery
	pkgPath string
	source  string
	path    string
}

// New returns an empty Handler.
func New() *Handler {
	return &Handler{caches: map[string]*policyCache{}}
}

// Mechanism returns the registry key.
func (h *Handler) Mechanism() string { return Mechanism }

// Prepare reads the entry's sibling .rego file, parses the package
// reference, and compiles a PreparedEvalQuery against
// `data.<package>`. The cache is refreshed on every call so a
// reloaded policy supersedes the previous version.
func (h *Handler) Prepare(ctx context.Context, entry policy_assessment.Entry) error {
	policyPath, err := resolvePolicyPath(entry)
	if err != nil {
		return err
	}
	src, err := os.ReadFile(policyPath)
	if err != nil {
		return fmt.Errorf("opa: read %s: %w", policyPath, err)
	}
	module, err := ast.ParseModule(policyPath, string(src))
	if err != nil {
		return fmt.Errorf("opa: parse %s: %w", policyPath, err)
	}
	if module.Package == nil || module.Package.Path == nil {
		return fmt.Errorf("opa: %s has no package declaration", policyPath)
	}
	pkgPath := module.Package.Path.String() // e.g. "data.lens.access_risk.orphaned_account"

	pq, err := rego.New(
		rego.Query(pkgPath),
		rego.Module(policyPath, string(src)),
	).PrepareForEval(ctx)
	if err != nil {
		return fmt.Errorf("opa: compile %s: %w", policyPath, err)
	}

	key := cacheKey(entry)
	h.mu.Lock()
	h.caches[key] = &policyCache{
		query:   pq,
		pkgPath: pkgPath,
		source:  string(src),
		path:    policyPath,
	}
	h.mu.Unlock()
	return nil
}

// Evaluate serialises Facts into a JSON-shaped input, runs the
// prepared Rego query, and maps the two contract output variables
// (decision, projected_facts) into a RuleResult.
func (h *Handler) Evaluate(ctx context.Context, req policy_assessment.Request) (policy_assessment.Output, error) {
	h.mu.RLock()
	cache, ok := h.caches[reqKey(req)]
	h.mu.RUnlock()
	if !ok {
		return policy_assessment.Output{}, fmt.Errorf("opa: policy not prepared: %s", req.PolicyID)
	}

	input, err := factsToInput(req.Facts)
	if err != nil {
		return policy_assessment.Output{}, fmt.Errorf("opa: marshal facts: %w", err)
	}

	rs, err := cache.query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return policy_assessment.Output{}, fmt.Errorf("opa: eval %s: %w", req.PolicyID, err)
	}
	if len(rs) == 0 || len(rs[0].Expressions) == 0 || rs[0].Expressions[0].Value == nil {
		return policy_assessment.Output{Matched: false}, nil
	}
	pkg, ok := rs[0].Expressions[0].Value.(map[string]any)
	if !ok {
		return policy_assessment.Output{}, fmt.Errorf("opa: %s: query result is not an object", req.PolicyID)
	}

	out := policy_assessment.Output{}

	if raw, has := pkg["decision"]; has && raw != nil {
		dmap, isMap := raw.(map[string]any)
		if isMap {
			dec, err := mapDecision(dmap)
			if err != nil {
				return policy_assessment.Output{}, fmt.Errorf("opa: %s decision: %w", req.PolicyID, err)
			}
			out.Result.Decision = dec
			out.Matched = true
		}
	}

	if raw, has := pkg["projected_facts"]; has && raw != nil {
		arr, isArr := raw.([]any)
		if isArr && len(arr) > 0 {
			pfs, err := mapProjectedFacts(arr)
			if err != nil {
				return policy_assessment.Output{}, fmt.Errorf("opa: %s projected_facts: %w", req.PolicyID, err)
			}
			out.Result.ProjectedFacts = pfs
			out.Matched = true
		}
	}

	return out, nil
}

// --- Facts → Rego input ---------------------------------------------

// factsToInput marshals Facts through JSON so the Rego rule sees the
// same snake_case shape that the kernel RULE_CONTRACT documents:
// input.principal, input.target, input.action, input.context, …
func factsToInput(f policy_assessment.Facts) (any, error) {
	raw, err := json.Marshal(f)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// --- Rego output → RuleResult ---------------------------------------

func mapDecision(m map[string]any) (*policy_assessment.Decision, error) {
	d := &policy_assessment.Decision{}
	if v, ok := m["effect"].(string); ok {
		d.Effect = v
	}
	if v, ok := m["risk_level"].(string); ok {
		d.RiskLevel = v
	}
	if raw, ok := m["signals"].([]any); ok {
		// Rule-level signals are polymorphic ([]any) per the kernel
		// contract — keep the values verbatim, no struct flattening.
		d.Signals = append(d.Signals, raw...)
	}
	if raw, ok := m["reasons"].([]any); ok {
		for _, item := range raw {
			rm, isMap := item.(map[string]any)
			if !isMap {
				continue
			}
			r, err := mapReason(rm)
			if err != nil {
				return nil, err
			}
			d.Reasons = append(d.Reasons, r)
		}
	}
	return d, nil
}

func mapReason(m map[string]any) (policy_assessment.Reason, error) {
	r := policy_assessment.Reason{}
	if v, ok := m["rule_id"].(string); ok {
		r.RuleID = v
	}
	if v, ok := m["rule_kind"].(string); ok {
		r.RuleKind = v
	}
	if v, ok := m["precedence"].(json.Number); ok {
		i, _ := v.Int64()
		r.Precedence = int(i)
	} else if v, ok := m["precedence"].(float64); ok {
		r.Precedence = int(v)
	}
	if v, ok := m["matched_conditions"].(map[string]any); ok {
		r.MatchedConditions = v
	}
	if v, ok := m["fact_values"].(map[string]any); ok {
		r.FactValues = v
	}
	if v, ok := m["produced"].(map[string]any); ok {
		r.Produced = v
	}
	return r, nil
}

func mapProjectedFacts(arr []any) ([]policy_assessment.ProjectedFact, error) {
	out := make([]policy_assessment.ProjectedFact, 0, len(arr))
	for _, item := range arr {
		pm, ok := item.(map[string]any)
		if !ok {
			continue
		}
		pf, err := mapProjectedFact(pm)
		if err != nil {
			return nil, err
		}
		out = append(out, pf)
	}
	return out, nil
}

func mapProjectedFact(m map[string]any) (policy_assessment.ProjectedFact, error) {
	pf := policy_assessment.ProjectedFact{}
	if v, ok := m["target"].(map[string]any); ok {
		// Re-encode through JSON to populate the typed struct from the
		// snake_case map keys.
		raw, err := json.Marshal(v)
		if err != nil {
			return pf, err
		}
		if err := json.Unmarshal(raw, &pf.Target); err != nil {
			return pf, err
		}
	}
	if v, ok := m["initiative"].(string); ok {
		pf.Initiative = v
	}
	if v, ok := m["valid_from"].(string); ok && v != "" {
		t, err := parseTimeRFC3339(v)
		if err != nil {
			return pf, fmt.Errorf("valid_from: %w", err)
		}
		pf.ValidFrom = &t
	}
	if v, ok := m["valid_until"].(string); ok && v != "" {
		t, err := parseTimeRFC3339(v)
		if err != nil {
			return pf, fmt.Errorf("valid_until: %w", err)
		}
		pf.ValidUntil = &t
	}
	if v, ok := m["desired_state"].(map[string]any); ok {
		ds := policy_assessment.DesiredState{}
		if b, ok := v["present"].(bool); ok {
			ds.Present = b
		}
		if attrs, ok := v["attributes"].(map[string]any); ok {
			ds.Attributes = attrs
		}
		pf.DesiredState = ds
	}
	if v, ok := m["risk_level"].(string); ok {
		pf.RiskLevel = v
	}
	if raw, ok := m["signals"].([]any); ok {
		pf.Signals = append(pf.Signals, raw...)
	}
	if raw, ok := m["reasons"].([]any); ok {
		for _, item := range raw {
			rm, isMap := item.(map[string]any)
			if !isMap {
				continue
			}
			r, err := mapReason(rm)
			if err != nil {
				return pf, err
			}
			pf.Reasons = append(pf.Reasons, r)
		}
	}
	return pf, nil
}

// --- helpers --------------------------------------------------------

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

// resolvePolicyPath finds the .rego file the handler should compile.
// Default: same directory as the .meta.json, basename = manifest
// basename minus the ".meta.json" suffix plus ".rego". An explicit
// override is honored via Body.policy_file (relative to BasePath dir
// or absolute).
func resolvePolicyPath(entry policy_assessment.Entry) (string, error) {
	if entry.Manifest.BasePath == "" {
		return "", errors.New("opa: entry has empty BasePath; cartridges provider must populate it")
	}
	if pf, ok := entry.Manifest.Body["policy_file"].(string); ok && pf != "" {
		if filepath.IsAbs(pf) {
			return pf, nil
		}
		return filepath.Join(filepath.Dir(entry.Manifest.BasePath), pf), nil
	}
	base := filepath.Base(entry.Manifest.BasePath)
	if filepath.Ext(base) == ".json" {
		base = base[:len(base)-len(".json")]
	}
	if filepath.Ext(base) == ".meta" {
		base = base[:len(base)-len(".meta")]
	}
	return filepath.Join(filepath.Dir(entry.Manifest.BasePath), base+".rego"), nil
}
