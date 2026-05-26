// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policy_assessment

import (
	"context"
	"testing"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
)

// countingHandler records how many times Evaluate ran, so the gate test
// can assert the mechanism was short-circuited.
type countingHandler struct {
	mechanism string
	calls     int
}

func (h *countingHandler) Mechanism() string                    { return h.mechanism }
func (h *countingHandler) Prepare(context.Context, Entry) error { return nil }
func (h *countingHandler) Evaluate(context.Context, Request) (Output, error) {
	h.calls++
	return Output{Matched: true}, nil
}

func gateEntry(requires ...string) Entry {
	return Entry{
		CartridgeRef: "ispm-data-quality",
		Manifest: cartridges.Manifest{
			RuleID:     "ispm_data_quality.evidence.missing_mfa_evidence",
			Mechanism:  "opa",
			StackCheck: &cartridges.StackCheck{Requires: requires},
		},
	}
}

func TestEvaluateEntry_StackCheck_NotEvaluableWhenEvidenceAbsent(t *testing.T) {
	h := &countingHandler{mechanism: "opa"}
	d := NewDispatcher()
	d.Register(h)

	// No EvidencePresent → required mfa_evidence is absent.
	out, err := d.EvaluateEntry(context.Background(), gateEntry("mfa_evidence"), Facts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.NotEvaluable {
		t.Fatal("want NotEvaluable=true when required evidence absent")
	}
	if out.Matched {
		t.Fatal("NotEvaluable and Matched are mutually exclusive")
	}
	if len(out.MissingEvidence) != 1 || out.MissingEvidence[0] != "mfa_evidence" {
		t.Fatalf("want MissingEvidence=[mfa_evidence], got %v", out.MissingEvidence)
	}
	if h.calls != 0 {
		t.Fatalf("mechanism must not run on not_evaluable, ran %d times", h.calls)
	}
}

func TestEvaluateEntry_StackCheck_DispatchesWhenEvidencePresent(t *testing.T) {
	h := &countingHandler{mechanism: "opa"}
	d := NewDispatcher()
	d.Register(h)

	facts := Facts{EvidencePresent: map[string]bool{"mfa_evidence": true}}
	out, err := d.EvaluateEntry(context.Background(), gateEntry("mfa_evidence"), facts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.NotEvaluable {
		t.Fatal("evidence present → must not be not_evaluable")
	}
	if h.calls != 1 {
		t.Fatalf("mechanism must run once when evaluable, ran %d", h.calls)
	}
}

func TestEvaluateEntry_NoStackCheck_DispatchesNormally(t *testing.T) {
	h := &countingHandler{mechanism: "opa"}
	d := NewDispatcher()
	d.Register(h)

	entry := Entry{
		CartridgeRef: "ispm-core-identity-posture",
		Manifest:     cartridges.Manifest{RuleID: "r", Mechanism: "opa"},
	}
	out, err := d.EvaluateEntry(context.Background(), entry, Facts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.NotEvaluable {
		t.Fatal("no stack_check → never not_evaluable")
	}
	if h.calls != 1 {
		t.Fatalf("want 1 dispatch, got %d", h.calls)
	}
}
