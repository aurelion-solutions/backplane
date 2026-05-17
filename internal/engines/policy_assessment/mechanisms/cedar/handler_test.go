// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package cedar

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
	"github.com/aurelion-solutions/backplane/internal/engines/policy_assessment"
)

const grantMatchPolicy = `permit (
    principal,
    action == Action::"view",
    resource == Document::"doc-42"
);
`

const denyInactivePolicy = `forbid (
    principal,
    action,
    resource
) when {
    principal.is_active == false
};
`

func writePolicy(t *testing.T, dir, base, body string) cartridges.Manifest {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	metaPath := filepath.Join(dir, base+".meta.json")
	if err := os.WriteFile(metaPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	cedarPath := filepath.Join(dir, base+".cedar")
	if err := os.WriteFile(cedarPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write cedar: %v", err)
	}
	return cartridges.Manifest{
		RuleID:    "sample." + base,
		Version:   1,
		Name:      base,
		Mechanism: Mechanism,
		BasePath:  metaPath,
	}
}

func TestCedar_Permit(t *testing.T) {
	dir := t.TempDir()
	manifest := writePolicy(t, dir, "grant_match", grantMatchPolicy)
	entry := policy_assessment.Entry{CartridgeRef: "sample", Manifest: manifest}

	h := New()
	if err := h.Prepare(context.Background(), entry); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	out, err := h.Evaluate(context.Background(), policy_assessment.Request{
		Mechanism:    Mechanism,
		PolicyID:     "sample/sample.grant_match",
		CartridgeRef: "sample",
		BasePath:     manifest.BasePath,
		Facts: policy_assessment.Facts{
			Principal: &policy_assessment.PrincipalFacts{ID: "alice", Kind: "Account"},
			Action:    "view",
			Resource:  &policy_assessment.Resource{Type: "Document", ID: "doc-42"},
			Now:       time.Now().UTC(),
		},
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if out.Result.Decision == nil || out.Result.Decision.Effect != policy_assessment.EffectAllow {
		t.Fatalf("expected allow, got %+v", out.Result.Decision)
	}
}

func TestCedar_DenyWhenInactiveFromPrincipalFacts(t *testing.T) {
	dir := t.TempDir()
	manifest := writePolicy(t, dir, "deny_inactive", denyInactivePolicy)
	entry := policy_assessment.Entry{CartridgeRef: "sample", Manifest: manifest}

	h := New()
	if err := h.Prepare(context.Background(), entry); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	out, err := h.Evaluate(context.Background(), policy_assessment.Request{
		Mechanism:    Mechanism,
		PolicyID:     "sample/sample.deny_inactive",
		CartridgeRef: "sample",
		BasePath:     manifest.BasePath,
		Facts: policy_assessment.Facts{
			// status != "active" → handler stamps is_active=false on
			// the auto-generated principal entity.
			Principal: &policy_assessment.PrincipalFacts{ID: "bob", Kind: "Account", Status: "disabled"},
			Action:    "view",
			Resource:  &policy_assessment.Resource{Type: "Document", ID: "x"},
			Now:       time.Now().UTC(),
		},
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if out.Result.Decision == nil || out.Result.Decision.Effect != policy_assessment.EffectDeny {
		t.Fatalf("expected deny, got %+v", out.Result.Decision)
	}
}

func TestCedar_NoMatchProducesNoDecision(t *testing.T) {
	dir := t.TempDir()
	manifest := writePolicy(t, dir, "grant_match", grantMatchPolicy)
	entry := policy_assessment.Entry{CartridgeRef: "sample", Manifest: manifest}

	h := New()
	_ = h.Prepare(context.Background(), entry)
	out, err := h.Evaluate(context.Background(), policy_assessment.Request{
		Mechanism:    Mechanism,
		PolicyID:     "sample/sample.grant_match",
		CartridgeRef: "sample",
		BasePath:     manifest.BasePath,
		Facts: policy_assessment.Facts{
			Principal: &policy_assessment.PrincipalFacts{ID: "alice", Kind: "Account"},
			Action:    "view",
			Resource:  &policy_assessment.Resource{Type: "Document", ID: "MISSING"},
			Now:       time.Now().UTC(),
		},
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if out.Matched {
		t.Fatalf("expected matched=false on no-match, got %+v", out)
	}
	if out.Result.Decision != nil {
		t.Fatalf("expected nil Decision on no-match, got %+v", out.Result.Decision)
	}
}

func TestCedar_ExplicitPolicyFile(t *testing.T) {
	dir := t.TempDir()
	metaPath := filepath.Join(dir, "x.meta.json")
	if err := os.WriteFile(metaPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	customPath := filepath.Join(dir, "custom_name.cedar")
	if err := os.WriteFile(customPath, []byte(grantMatchPolicy), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := cartridges.Manifest{
		RuleID:    "sample.custom",
		Mechanism: Mechanism,
		BasePath:  metaPath,
		Body:      map[string]any{"policy_file": "custom_name.cedar"},
	}
	entry := policy_assessment.Entry{CartridgeRef: "sample", Manifest: manifest}
	h := New()
	if err := h.Prepare(context.Background(), entry); err != nil {
		t.Fatalf("prepare with explicit policy_file: %v", err)
	}
}
