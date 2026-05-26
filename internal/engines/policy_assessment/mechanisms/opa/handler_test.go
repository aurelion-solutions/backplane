// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package opa

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
	"github.com/aurelion-solutions/backplane/internal/engines/policy_assessment"
)

func writePolicy(t *testing.T, dir, base, body string) cartridges.Manifest {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	metaPath := filepath.Join(dir, base+".meta.json")
	if err := os.WriteFile(metaPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	regoPath := filepath.Join(dir, base+".rego")
	if err := os.WriteFile(regoPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write rego: %v", err)
	}
	return cartridges.Manifest{
		RuleID:    "sample." + base,
		Version:   1,
		Name:      base,
		Mechanism: Mechanism,
		BasePath:  metaPath,
	}
}

func evalEntry(t *testing.T, h *Handler, entry policy_assessment.Entry, facts policy_assessment.Facts) policy_assessment.Output {
	t.Helper()
	if err := h.Prepare(context.Background(), entry); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	out, err := h.Evaluate(context.Background(), policy_assessment.Request{
		Mechanism:    Mechanism,
		PolicyID:     entry.CartridgeRef + "/" + entry.Manifest.RuleID,
		CartridgeRef: entry.CartridgeRef,
		BasePath:     entry.Manifest.BasePath,
		Facts:        facts,
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	return out
}

const orphanAccountRego = `package sample.orphan_account_recent_login

import rego.v1

year_ns := 365 * 24 * 3600 * 1000000000

default decision := null

decision := {
	"risk_level": "high",
	"signals":    ["orphaned_account_recent_login", {"kind": "marker", "extra": 1}],
	"reasons": [{
		"rule_id":   "sample/orphan_account_recent_login",
		"rule_kind": "anomaly",
		"matched_conditions": {
			"target.principal_id":   "null",
			"target.last_login_at":  "< now - 1y",
		},
		"fact_values": {
			"target.last_login_at": input.target.last_login_at,
		},
	}],
} if {
	input.target.kind == "account"
	not input.target.principal_id
	input.target.last_login_at
	time.parse_rfc3339_ns(input.target.last_login_at) > time.now_ns() - year_ns
}
`

const allowGateRego = `package sample.allow_active

import rego.v1

default decision := null

decision := {
	"effect": "allow",
	"signals": ["active_login_allowed"],
	"reasons": [{
		"rule_id":   "sample/allow_active",
		"rule_kind": "reactive_gate",
	}],
} if {
	input.principal.status == "active"
	input.action == "login"
}
`

const birthrightRego = `package sample.rd_birthright

import rego.v1

default projected_facts := []

projected_facts := [
	{
		"target":        {"application": "jira", "resource_type": "account", "resource": "primary"},
		"initiative":    "birthright",
		"desired_state": {"present": true, "attributes": {}},
		"signals":       ["birthright_rd_jira"],
		"reasons": [{
			"rule_id":            "sample/rd_birthright",
			"rule_kind":          "generative_birthright",
			"matched_conditions": {"principal_context.attributes.department": "R&D"},
		}],
	},
	{
		"target":        {"application": "slack", "resource_type": "account", "resource": "primary"},
		"initiative":    "birthright",
		"desired_state": {"present": true, "attributes": {}},
		"signals":       ["birthright_rd_slack"],
		"reasons": [{
			"rule_id": "sample/rd_birthright",
		}],
	},
] if {
	input.principal.status == "active"
	input.principal_context.attributes.department == "R&D"
}
`

// TestOPA_Allow checks the reactive gate path: rule fires and emits
// Decision.Effect=allow.
func TestOPA_Allow(t *testing.T) {
	dir := t.TempDir()
	manifest := writePolicy(t, dir, "allow_active", allowGateRego)
	entry := policy_assessment.Entry{CartridgeRef: "sample", Manifest: manifest}
	out := evalEntry(t, New(), entry, policy_assessment.Facts{
		Principal: &policy_assessment.PrincipalFacts{ID: "alice", Status: "active"},
		Action:    "login",
		Now:       time.Now().UTC(),
	})
	if !out.Matched {
		t.Fatalf("expected matched=true; out=%+v", out)
	}
	if out.Result.Decision == nil || out.Result.Decision.Effect != policy_assessment.EffectAllow {
		t.Fatalf("expected allow; decision=%+v", out.Result.Decision)
	}
	if len(out.Result.Decision.Signals) != 1 {
		t.Fatalf("expected 1 signal; got %v", out.Result.Decision.Signals)
	}
	if s, ok := out.Result.Decision.Signals[0].(string); !ok || s != "active_login_allowed" {
		t.Fatalf("expected string signal; got %T %v", out.Result.Decision.Signals[0], out.Result.Decision.Signals[0])
	}
}

// TestOPA_Anomaly checks the reactive anomaly path: orphan account
// finding with risk_level + polymorphic signals (string + dict mix).
func TestOPA_Anomaly(t *testing.T) {
	dir := t.TempDir()
	manifest := writePolicy(t, dir, "orphan_account_recent_login", orphanAccountRego)
	entry := policy_assessment.Entry{CartridgeRef: "sample", Manifest: manifest}

	t0 := time.Now().UTC().Add(-30 * 24 * time.Hour)
	out := evalEntry(t, New(), entry, policy_assessment.Facts{
		Target: &policy_assessment.TargetFacts{
			Kind:        "account",
			Application: "salesforce",
			// PrincipalID intentionally nil → orphan
			LastLoginAt: &t0,
		},
		Now: time.Now().UTC(),
	})
	if !out.Matched {
		t.Fatalf("expected matched=true; out=%+v", out)
	}
	if out.Result.Decision == nil {
		t.Fatalf("expected Decision; out=%+v", out)
	}
	if out.Result.Decision.Effect != "" {
		t.Fatalf("anomaly: Effect should be empty; got %q", out.Result.Decision.Effect)
	}
	if out.Result.Decision.RiskLevel != policy_assessment.RiskHigh {
		t.Fatalf("expected risk_level=high; got %q", out.Result.Decision.RiskLevel)
	}
	if len(out.Result.Decision.Signals) != 2 {
		t.Fatalf("expected 2 signals; got %v", out.Result.Decision.Signals)
	}
	// Polymorphic preservation: first is string, second is map.
	if _, ok := out.Result.Decision.Signals[0].(string); !ok {
		t.Fatalf("signal[0] should be string; got %T", out.Result.Decision.Signals[0])
	}
	if _, ok := out.Result.Decision.Signals[1].(map[string]any); !ok {
		t.Fatalf("signal[1] should be map; got %T", out.Result.Decision.Signals[1])
	}
}

// TestOPA_Generative checks the generative path: projected_facts
// non-empty, Decision nil.
func TestOPA_Generative(t *testing.T) {
	dir := t.TempDir()
	manifest := writePolicy(t, dir, "rd_birthright", birthrightRego)
	entry := policy_assessment.Entry{CartridgeRef: "sample", Manifest: manifest}

	out := evalEntry(t, New(), entry, policy_assessment.Facts{
		Principal: &policy_assessment.PrincipalFacts{ID: "emp-77", Status: "active"},
		PrincipalContext: &policy_assessment.PrincipalContextFacts{
			Attributes: map[string]any{"department": "R&D"},
		},
		Now: time.Now().UTC(),
	})
	if !out.Matched {
		t.Fatalf("expected matched=true; out=%+v", out)
	}
	if out.Result.Decision != nil {
		t.Fatalf("generative: Decision should be nil; got %+v", out.Result.Decision)
	}
	if len(out.Result.ProjectedFacts) != 2 {
		t.Fatalf("expected 2 projected facts; got %d", len(out.Result.ProjectedFacts))
	}
	if pf := out.Result.ProjectedFacts[0]; pf.Target.Application != "jira" || pf.Initiative != policy_assessment.InitiativeBirthright {
		t.Fatalf("pf[0] mismatch: %+v", pf)
	}
	if pf := out.Result.ProjectedFacts[1]; pf.Target.Application != "slack" {
		t.Fatalf("pf[1] mismatch: %+v", pf)
	}
}

// TestOPA_NoMatch checks the "rule did not fire" path: Decision nil,
// ProjectedFacts empty, Matched=false.
func TestOPA_NoMatch(t *testing.T) {
	dir := t.TempDir()
	manifest := writePolicy(t, dir, "allow_active", allowGateRego)
	entry := policy_assessment.Entry{CartridgeRef: "sample", Manifest: manifest}

	out := evalEntry(t, New(), entry, policy_assessment.Facts{
		Principal: &policy_assessment.PrincipalFacts{ID: "bob", Status: "disabled"},
		Action:    "login",
		Now:       time.Now().UTC(),
	})
	if out.Matched {
		t.Fatalf("expected matched=false on no-match; out=%+v", out)
	}
	if out.Result.Decision != nil {
		t.Fatalf("expected nil Decision on no-match; got %+v", out.Result.Decision)
	}
	if len(out.Result.ProjectedFacts) != 0 {
		t.Fatalf("expected empty ProjectedFacts; got %v", out.Result.ProjectedFacts)
	}
}

// privilegedRobustRego mirrors the fixed ispm-core privileged_access:
// it matches on account-level privilege but builds its decision object
// using object.get for the optional Path-2 fields, so an account-level
// match (no action / privilege_level in the envelope) still produces a
// decision.
const privilegedRobustRego = `package sample.privileged_robust

import rego.v1

default decision := null

matched if { input.target.account_is_privileged == true }
matched if { input.action == "administer"; input.target.privilege_level == "admin" }

decision := {
	"risk_level": "high",
	"signals":    ["privileged_access"],
	"reasons": [{
		"rule_id":   "sample/privileged_robust",
		"rule_kind": "access_risk",
		"fact_values": {
			"target.account_is_privileged": input.target.account_is_privileged,
			"action":                       object.get(input, "action", ""),
			"target.privilege_level":       object.get(input.target, "privilege_level", ""),
		},
	}],
} if { matched }
`

// privilegedEagerRego is the pre-fix shape: it references input.action
// directly while constructing the decision. On an account-level match
// with no action present, that undefined reference undefines the whole
// rule and the decision silently falls back to the null default.
const privilegedEagerRego = `package sample.privileged_eager

import rego.v1

default decision := null

matched if { input.target.account_is_privileged == true }

decision := {
	"risk_level": "high",
	"signals":    ["privileged_access"],
	"reasons": [{"fact_values": {"action": input.action}}],
} if { matched }
`

// TestOPA_RobustDecisionFiresWithoutOptionalFields locks the
// privileged_access fix: object.get keeps the decision defined when an
// optional input field is absent, where a direct reference would not.
func TestOPA_RobustDecisionFiresWithoutOptionalFields(t *testing.T) {
	priv := true
	facts := policy_assessment.Facts{
		Target: &policy_assessment.TargetFacts{
			Kind:                "account",
			ID:                  "acc-1",
			AccountIsPrivileged: &priv,
			// No action / privilege_level — an account-level match.
		},
		Now: time.Now().UTC(),
	}

	robust := writePolicy(t, t.TempDir(), "privileged_robust", privilegedRobustRego)
	out := evalEntry(t, New(), policy_assessment.Entry{CartridgeRef: "sample", Manifest: robust}, facts)
	if !out.Matched || out.Result.Decision == nil {
		t.Fatalf("robust policy should fire on account-level privilege; out=%+v", out)
	}
	if out.Result.Decision.RiskLevel != policy_assessment.RiskHigh {
		t.Fatalf("expected risk_level=high; got %q", out.Result.Decision.RiskLevel)
	}

	eager := writePolicy(t, t.TempDir(), "privileged_eager", privilegedEagerRego)
	outEager := evalEntry(t, New(), policy_assessment.Entry{CartridgeRef: "sample", Manifest: eager}, facts)
	if outEager.Matched || outEager.Result.Decision != nil {
		t.Fatalf("eager policy must NOT fire (undefined input.action undefines the rule); out=%+v", outEager)
	}
}

// TestOPA_ExplicitPolicyFile honours Body.policy_file override.
func TestOPA_ExplicitPolicyFile(t *testing.T) {
	dir := t.TempDir()
	metaPath := filepath.Join(dir, "x.meta.json")
	if err := os.WriteFile(metaPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	customPath := filepath.Join(dir, "custom_name.rego")
	if err := os.WriteFile(customPath, []byte(allowGateRego), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := cartridges.Manifest{
		RuleID:    "sample.custom",
		Mechanism: Mechanism,
		BasePath:  metaPath,
		Body:      map[string]any{"policy_file": "custom_name.rego"},
	}
	entry := policy_assessment.Entry{CartridgeRef: "sample", Manifest: manifest}
	h := New()
	if err := h.Prepare(context.Background(), entry); err != nil {
		t.Fatalf("prepare with explicit policy_file: %v", err)
	}
}
