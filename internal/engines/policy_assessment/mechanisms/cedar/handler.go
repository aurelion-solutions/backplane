// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package cedar

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/aurelion-solutions/backplane/internal/engines/policy_assessment"
	cedargo "github.com/cedar-policy/cedar-go"
	cedartypes "github.com/cedar-policy/cedar-go/types"
)

// Mechanism is the policy_assessment registry key.
const Mechanism = "cedar"

// Handler is the cedar mechanism handler. One handler instance serves
// every cedar policy in the catalogue; per-policy state (compiled
// PolicySet) is cached internally.
type Handler struct {
	mu     sync.RWMutex
	caches map[string]*policyCache // keyed by Entry.CartridgeRef + "/" + RuleID
}

// policyCache holds the loaded Cedar policy + the source path for
// diagnostics.
type policyCache struct {
	source []byte
	set    atomic.Pointer[cedargo.PolicySet]
	path   string
}

// New returns an empty Handler.
func New() *Handler {
	return &Handler{caches: map[string]*policyCache{}}
}

// Mechanism returns the registry key.
func (h *Handler) Mechanism() string { return Mechanism }

// Prepare loads / reloads the Cedar policy text from the entry's
// sibling .cedar file and compiles it.
func (h *Handler) Prepare(_ context.Context, entry policy_assessment.Entry) error {
	policyPath, err := resolvePolicyPath(entry)
	if err != nil {
		return err
	}
	src, err := os.ReadFile(policyPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", policyPath, err)
	}
	set, err := cedargo.NewPolicySetFromBytes(filepath.Base(policyPath), src)
	if err != nil {
		return fmt.Errorf("compile %s: %w", policyPath, err)
	}

	key := cacheKey(entry)
	h.mu.Lock()
	cache, ok := h.caches[key]
	if !ok {
		cache = &policyCache{}
		h.caches[key] = cache
	}
	h.mu.Unlock()
	cache.source = src
	cache.path = policyPath
	cache.set.Store(set)
	return nil
}

// Evaluate maps Facts → Cedar Request + EntityMap, runs IsAuthorized,
// and translates the result back into a policy_assessment.Output with
// a proper RuleResult.Decision.
func (h *Handler) Evaluate(_ context.Context, req policy_assessment.Request) (policy_assessment.Output, error) {
	h.mu.RLock()
	cache, ok := h.caches[reqKey(req)]
	h.mu.RUnlock()
	if !ok {
		return policy_assessment.Output{}, fmt.Errorf("cedar: policy not prepared: %s", req.PolicyID)
	}
	set := cache.set.Load()
	if set == nil {
		return policy_assessment.Output{}, fmt.Errorf("cedar: policy set empty: %s", req.PolicyID)
	}

	principal, err := principalUID(req.Facts)
	if err != nil {
		return policy_assessment.Output{}, err
	}
	resource, err := resourceUID(req.Facts)
	if err != nil {
		return policy_assessment.Output{}, err
	}
	action := actionUID(req.Facts)

	cedarReq := cedargo.Request{
		Principal: principal,
		Action:    action,
		Resource:  resource,
		Context:   contextRecord(req.Facts),
	}

	entities := buildEntities(req.Facts)
	decision, diag := set.IsAuthorized(entities, cedarReq)

	// Cedar IsAuthorized() semantics:
	//
	//   Allow + Reasons!=∅   → a permit policy fired
	//   Deny  + Reasons==∅   → no policy fired (default deny) — NOT a
	//                          mechanism verdict, just absence of input
	//   Deny  + Reasons!=∅   → a forbid policy fired
	//
	// We map the third to a real deny, the first to allow, and the
	// second to "not applicable" so aggregation callers don't treat
	// absence as an explicit decision.
	reasons := make([]policy_assessment.Reason, 0, len(diag.Reasons))
	for _, r := range diag.Reasons {
		reasons = append(reasons, policy_assessment.Reason{
			RuleID:   req.PolicyID,
			RuleKind: "cedar",
			Produced: map[string]any{
				"cedar_policy_id": string(r.PolicyID),
				"position":        map[string]any{"line": r.Position.Line, "column": r.Position.Column},
			},
		})
	}
	evidence := make([]policy_assessment.Evidence, 0, len(diag.Errors))
	for _, e := range diag.Errors {
		evidence = append(evidence, policy_assessment.Evidence{
			SourceType: "cedar_error",
			SourceID:   string(e.PolicyID),
			Summary:    e.Message,
		})
	}

	out := policy_assessment.Output{Evidence: evidence}
	switch {
	case bool(decision) && len(diag.Reasons) > 0:
		out.Matched = true
		out.Result.Decision = &policy_assessment.Decision{
			Effect:  policy_assessment.EffectAllow,
			Reasons: reasons,
		}
	case !bool(decision) && len(diag.Reasons) > 0:
		out.Matched = true
		out.Result.Decision = &policy_assessment.Decision{
			Effect:  policy_assessment.EffectDeny,
			Reasons: reasons,
		}
	default:
		out.Matched = len(diag.Errors) > 0
	}
	return out, nil
}

// Forget removes a cached policy. Used by the store reload path when an
// entry disappears from the catalogue.
func (h *Handler) Forget(entry policy_assessment.Entry) {
	h.mu.Lock()
	delete(h.caches, cacheKey(entry))
	h.mu.Unlock()
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

// resolvePolicyPath finds the .cedar file the handler should compile.
// Default: same directory as the .meta.json, basename = manifest
// basename minus the ".meta.json" suffix plus ".cedar". An explicit
// override is honored via Body.policy_file (relative to BasePath dir
// or absolute).
func resolvePolicyPath(entry policy_assessment.Entry) (string, error) {
	if entry.Manifest.BasePath == "" {
		return "", errors.New("cedar: entry has empty BasePath; cartridges provider must populate it")
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
	return filepath.Join(filepath.Dir(entry.Manifest.BasePath), base+".cedar"), nil
}

// --- Facts → Cedar conversions --------------------------------------

// principalUID derives the Cedar principal from Facts.Principal. Kind
// (e.g. "Account", "Principal", "Workload") becomes the Cedar entity
// type; ID becomes the Cedar entity ID. Falls back to "Principal"
// when Kind is empty.
func principalUID(f policy_assessment.Facts) (cedartypes.EntityUID, error) {
	if f.Principal == nil || f.Principal.ID == "" {
		return cedartypes.EntityUID{}, errors.New("cedar: facts.principal.id is required")
	}
	typ := f.Principal.Kind
	if typ == "" {
		typ = "Principal"
	}
	return cedargo.NewEntityUID(cedartypes.EntityType(typ), cedartypes.String(f.Principal.ID)), nil
}

// resourceUID prefers the typed Facts.Resource (used by AuthZ
// transports) and falls back to Facts.Target.ResourceType+.Resource
// when only a TargetFacts shape is supplied. Empty resource = sentinel
// "Resource::\"\"" so Cedar policies that ignore resource still match.
func resourceUID(f policy_assessment.Facts) (cedartypes.EntityUID, error) {
	if f.Resource != nil && f.Resource.Type != "" {
		return cedargo.NewEntityUID(
			cedartypes.EntityType(f.Resource.Type),
			cedartypes.String(f.Resource.ID),
		), nil
	}
	if f.Target != nil && f.Target.ResourceType != "" {
		return cedargo.NewEntityUID(
			cedartypes.EntityType(f.Target.ResourceType),
			cedartypes.String(f.Target.Resource),
		), nil
	}
	return cedargo.NewEntityUID("Resource", ""), nil
}

// actionUID maps Facts.Action (a verb name) into the conventional
// `Action::"<name>"` Cedar UID.
func actionUID(f policy_assessment.Facts) cedartypes.EntityUID {
	return cedargo.NewEntityUID("Action", cedartypes.String(f.Action))
}

// contextRecord builds the Cedar Context Record. Pulls transport /
// country / IP from ContextFacts plus the Threat block flattened into
// keys the rule can read (`threat.risk_score`, `threat.indicators`,
// etc.) and the open-ended Context.Extra.
func contextRecord(f policy_assessment.Facts) cedartypes.Record {
	m := cedartypes.RecordMap{}
	if f.Context != nil {
		if f.Context.Transport != "" {
			m["transport"] = cedartypes.String(f.Context.Transport)
		}
		if f.Context.Country != "" {
			m["country"] = cedartypes.String(f.Context.Country)
		}
		if f.Context.IP != "" {
			m["ip"] = cedartypes.String(f.Context.IP)
		}
		for k, v := range f.Context.Extra {
			m[cedartypes.String(k)] = toCedarValue(v)
		}
	}
	if f.Threat != nil {
		threatMap := map[string]any{}
		if f.Threat.RiskScore != nil {
			threatMap["risk_score"] = *f.Threat.RiskScore
		}
		if len(f.Threat.ActiveIndicators) > 0 {
			indicators := make([]any, len(f.Threat.ActiveIndicators))
			for i, s := range f.Threat.ActiveIndicators {
				indicators[i] = s
			}
			threatMap["active_indicators"] = indicators
		}
		if f.Threat.UEBARiskScore != nil {
			threatMap["ueba_risk_score"] = *f.Threat.UEBARiskScore
		}
		if f.Threat.BehavioralAnomaly != nil {
			threatMap["behavioral_anomaly"] = *f.Threat.BehavioralAnomaly
		}
		if len(threatMap) > 0 {
			m["threat"] = toCedarValue(threatMap)
		}
	}
	for k, v := range f.Extra {
		m[cedartypes.String(k)] = toCedarValue(v)
	}
	return cedartypes.NewRecord(m)
}

// buildEntities composes the Cedar EntityMap from Facts.Entities plus
// an auto-generated entity for Principal (so attributes from
// PrincipalFacts surface in policy `principal.attr` reads even when the
// caller didn't supply an explicit entity record).
func buildEntities(f policy_assessment.Facts) cedartypes.EntityMap {
	out := cedartypes.EntityMap{}
	for _, e := range f.Entities {
		uid := cedargo.NewEntityUID(cedartypes.EntityType(e.UID.Type), cedartypes.String(e.UID.ID))
		parents := make([]cedartypes.EntityUID, 0, len(e.Parents))
		for _, p := range e.Parents {
			parents = append(parents, cedargo.NewEntityUID(cedartypes.EntityType(p.Type), cedartypes.String(p.ID)))
		}
		out[uid] = cedartypes.Entity{
			UID:        uid,
			Parents:    cedartypes.NewEntityUIDSet(parents...),
			Attributes: toCedarRecord(e.Attrs),
		}
	}
	if f.Principal != nil && f.Principal.ID != "" {
		typ := f.Principal.Kind
		if typ == "" {
			typ = "Principal"
		}
		uid := cedargo.NewEntityUID(cedartypes.EntityType(typ), cedartypes.String(f.Principal.ID))
		if _, exists := out[uid]; !exists {
			attrs := principalAttrs(f.Principal)
			out[uid] = cedartypes.Entity{
				UID:        uid,
				Parents:    cedartypes.NewEntityUIDSet(),
				Attributes: attrs,
			}
		}
	}
	return out
}

// principalAttrs flattens PrincipalFacts into a Cedar Record so policies
// can read fields like `principal.is_active`, `principal.tenant_id`,
// `principal.mfa_enabled` directly.
func principalAttrs(s *policy_assessment.PrincipalFacts) cedartypes.Record {
	m := cedartypes.RecordMap{}
	if s.Status != "" {
		m["status"] = cedartypes.String(s.Status)
		m["is_active"] = cedartypes.Boolean(s.Status == "active")
	}
	if s.OrgUnit != "" {
		m["org_unit"] = cedartypes.String(s.OrgUnit)
	}
	if s.MFAEnabled != nil {
		m["mfa_enabled"] = cedartypes.Boolean(*s.MFAEnabled)
	}
	if s.EmailVerified != nil {
		m["email_verified"] = cedartypes.Boolean(*s.EmailVerified)
	}
	if s.TenantID != "" {
		m["tenant_id"] = cedartypes.String(s.TenantID)
	}
	if s.TenantRole != "" {
		m["tenant_role"] = cedartypes.String(s.TenantRole)
	}
	if s.PlanTier != "" {
		m["plan_tier"] = cedartypes.String(s.PlanTier)
	}
	for k, v := range s.Attributes {
		m[cedartypes.String(k)] = toCedarValue(v)
	}
	return cedartypes.NewRecord(m)
}

// --- generic Go any → Cedar Value conversion ------------------------

func toCedarRecord(in map[string]any) cedartypes.Record {
	out := make(cedartypes.RecordMap, len(in))
	for k, v := range in {
		out[cedartypes.String(k)] = toCedarValue(v)
	}
	return cedartypes.NewRecord(out)
}

func toCedarValue(v any) cedartypes.Value {
	switch x := v.(type) {
	case string:
		return cedartypes.String(x)
	case bool:
		return cedartypes.Boolean(x)
	case int:
		return cedartypes.Long(int64(x))
	case int64:
		return cedartypes.Long(x)
	case float64:
		return cedartypes.Long(int64(x))
	case map[string]any:
		return toCedarRecord(x)
	case []any:
		set := make([]cedartypes.Value, 0, len(x))
		for _, item := range x {
			set = append(set, toCedarValue(item))
		}
		return cedartypes.NewSet(set...)
	case nil:
		return cedartypes.String("")
	default:
		b, _ := json.Marshal(x)
		return cedartypes.String(string(b))
	}
}
