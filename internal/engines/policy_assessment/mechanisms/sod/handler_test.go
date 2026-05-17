// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package sod

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
	"github.com/aurelion-solutions/backplane/internal/engines/policy_assessment"
)

func mkManifest(ruleID string, body map[string]any) cartridges.Manifest {
	return cartridges.Manifest{
		RuleID:    ruleID,
		Version:   1,
		Name:      ruleID,
		Mechanism: Mechanism,
		Body:      body,
	}
}

func mkEntry(ruleID string, body map[string]any) policy_assessment.Entry {
	return policy_assessment.Entry{
		CartridgeRef: "sample",
		Manifest:     mkManifest(ruleID, body),
	}
}

func mkRequest(entry policy_assessment.Entry, slugs []string) policy_assessment.Request {
	return policy_assessment.Request{
		Mechanism:    Mechanism,
		PolicyID:     entry.CartridgeRef + "/" + entry.Manifest.RuleID,
		CartridgeRef: entry.CartridgeRef,
		BasePath:     entry.Manifest.BasePath,
		Body:         entry.Manifest.Body,
		Facts: policy_assessment.Facts{
			Principal: &policy_assessment.PrincipalFacts{
				ID:              "alice",
				Kind:            "Person",
				CapabilitySlugs: slugs,
			},
			Now: time.Now().UTC(),
		},
	}
}

// TestSoD_AllConditionsMet — payments.creator + payments.approver
// both held → conflict surfaces with risk_level=high and the
// structured signal payload listing both matched slugs.
func TestSoD_AllConditionsMet(t *testing.T) {
	entry := mkEntry("sod.payments.no_self_approval", map[string]any{
		"conditions": []any{
			map[string]any{"capability_slugs": []any{"payments.creator"}, "min_count": 1},
			map[string]any{"capability_slugs": []any{"payments.approver"}, "min_count": 1},
		},
	})
	h := New()
	if err := h.Prepare(context.Background(), entry); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	out, err := h.Evaluate(context.Background(), mkRequest(entry, []string{
		"payments.creator", "payments.approver", "general.read",
	}))
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if !out.Matched {
		t.Fatalf("expected matched=true; out=%+v", out)
	}
	dec := out.Result.Decision
	if dec == nil {
		t.Fatalf("expected Decision; got nil")
	}
	if dec.Effect != "" {
		t.Fatalf("anomaly: Effect must stay empty; got %q", dec.Effect)
	}
	if dec.RiskLevel != policy_assessment.RiskHigh {
		t.Fatalf("expected risk_level=high; got %q", dec.RiskLevel)
	}
	if len(dec.Signals) != 2 {
		t.Fatalf("expected 2 signals; got %v", dec.Signals)
	}
	if _, ok := dec.Signals[0].(string); !ok {
		t.Fatalf("signal[0] must be a string code; got %T", dec.Signals[0])
	}
	conflict, ok := dec.Signals[1].(map[string]any)
	if !ok {
		t.Fatalf("signal[1] must be a structured dict; got %T", dec.Signals[1])
	}
	if conflict["kind"] != "sod_conflict" || conflict["principal"] != "alice" {
		t.Fatalf("signal payload mismatch: %+v", conflict)
	}
}

// TestSoD_ConditionShortfall — principal holds only one of the two
// required capability families → rule does not fire.
func TestSoD_ConditionShortfall(t *testing.T) {
	entry := mkEntry("sod.payments.no_self_approval", map[string]any{
		"conditions": []any{
			map[string]any{"capability_slugs": []any{"payments.creator"}, "min_count": 1},
			map[string]any{"capability_slugs": []any{"payments.approver"}, "min_count": 1},
		},
	})
	h := New()
	_ = h.Prepare(context.Background(), entry)
	out, _ := h.Evaluate(context.Background(), mkRequest(entry, []string{"payments.creator"}))
	if out.Matched {
		t.Fatalf("expected matched=false; got %+v", out)
	}
	if out.Result.Decision != nil {
		t.Fatalf("expected nil Decision on shortfall; got %+v", out.Result.Decision)
	}
}

// TestSoD_MinCountGreaterThanOne — condition wants >= 2 of a family;
// holding one of three options is not enough.
func TestSoD_MinCountGreaterThanOne(t *testing.T) {
	entry := mkEntry("sod.multi_admin", map[string]any{
		"conditions": []any{
			map[string]any{
				"capability_slugs": []any{"admin.a", "admin.b", "admin.c"},
				"min_count":        2,
			},
		},
	})
	h := New()
	_ = h.Prepare(context.Background(), entry)

	out, _ := h.Evaluate(context.Background(), mkRequest(entry, []string{"admin.a"}))
	if out.Matched {
		t.Fatalf("min_count=2 with one slug should not fire")
	}

	out, _ = h.Evaluate(context.Background(), mkRequest(entry, []string{"admin.a", "admin.b"}))
	if !out.Matched {
		t.Fatalf("min_count=2 with two slugs should fire; got %+v", out)
	}
}

// TestSoD_EmptyPrincipal — no principal or no slugs → no fire (does
// not panic).
func TestSoD_EmptyPrincipal(t *testing.T) {
	entry := mkEntry("sod.payments.no_self_approval", map[string]any{
		"conditions": []any{
			map[string]any{"capability_slugs": []any{"payments.creator"}, "min_count": 1},
		},
	})
	h := New()
	_ = h.Prepare(context.Background(), entry)

	// nil principal
	out, _ := h.Evaluate(context.Background(), policy_assessment.Request{
		Mechanism:    Mechanism,
		PolicyID:     "sample/sod.payments.no_self_approval",
		CartridgeRef: "sample",
		Body:         entry.Manifest.Body,
		Facts:        policy_assessment.Facts{Now: time.Now().UTC()},
	})
	if out.Matched {
		t.Fatalf("nil principal must not fire")
	}

	// empty slugs
	out, _ = h.Evaluate(context.Background(), mkRequest(entry, nil))
	if out.Matched {
		t.Fatalf("empty slugs must not fire")
	}
}

// TestSoD_PrepareRejectsBadBody — missing / empty conditions block
// must be rejected at Prepare so the catalogue never ships a rule
// that can never fire.
func TestSoD_PrepareRejectsBadBody(t *testing.T) {
	h := New()
	cases := []struct {
		name string
		body map[string]any
	}{
		{"no conditions key", map[string]any{}},
		{"empty conditions", map[string]any{"conditions": []any{}}},
		{"condition with no slugs", map[string]any{
			"conditions": []any{
				map[string]any{"capability_slugs": []any{}, "min_count": 1},
			},
		}},
		{"condition with zero min_count", map[string]any{
			"conditions": []any{
				map[string]any{"capability_slugs": []any{"x"}, "min_count": 0},
			},
		}},
	}
	for _, tc := range cases {
		err := h.Prepare(context.Background(), mkEntry("bad."+tc.name, tc.body))
		if err == nil {
			t.Fatalf("%s: expected error", tc.name)
		}
		if !strings.HasPrefix(err.Error(), "sod:") {
			t.Fatalf("%s: error should be prefixed sod:; got %q", tc.name, err.Error())
		}
	}
}
