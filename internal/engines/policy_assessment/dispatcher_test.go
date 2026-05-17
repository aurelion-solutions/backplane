// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policy_assessment

import (
	"context"
	"errors"
	"testing"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
)

type stubHandler struct {
	mechanism string
	prepared  []string
	prepErr   error
	output    Output
	evalErr   error
}

func (h *stubHandler) Mechanism() string { return h.mechanism }
func (h *stubHandler) Prepare(_ context.Context, e Entry) error {
	h.prepared = append(h.prepared, e.Manifest.RuleID)
	return h.prepErr
}
func (h *stubHandler) Evaluate(_ context.Context, _ Request) (Output, error) {
	return h.output, h.evalErr
}

func TestDispatcher_RouteByMechanism(t *testing.T) {
	cedarH := &stubHandler{mechanism: "cedar", output: Output{Matched: true,
		Result: RuleResult{Decision: &Decision{
			Effect:  EffectAllow,
			Reasons: []Reason{{RuleID: "alpha/r1"}},
		}}}}
	sodH := &stubHandler{mechanism: "sod", output: Output{Matched: true,
		Result: RuleResult{Decision: &Decision{RiskLevel: RiskHigh}}}}

	d := NewDispatcher()
	d.Register(cedarH)
	d.Register(sodH)

	if !d.Has("cedar") || !d.Has("sod") {
		t.Fatalf("register: %v", d.Mechanisms())
	}

	cedarOut, err := d.Evaluate(context.Background(), Request{Mechanism: "cedar"})
	if err != nil {
		t.Fatalf("cedar: %v", err)
	}
	if cedarOut.Result.Decision.Effect != "allow" {
		t.Fatalf("cedar effect=%s", cedarOut.Result.Decision.Effect)
	}
	sodOut, err := d.Evaluate(context.Background(), Request{Mechanism: "sod"})
	if err != nil {
		t.Fatalf("sod: %v", err)
	}
	if sodOut.Result.Decision.RiskLevel != "high" {
		t.Fatalf("sod risk=%s", sodOut.Result.Decision.RiskLevel)
	}
}

func TestDispatcher_UnknownMechanism(t *testing.T) {
	d := NewDispatcher()
	_, err := d.Evaluate(context.Background(), Request{Mechanism: "nosuch"})
	if !errors.Is(err, ErrUnknownMechanism) {
		t.Fatalf("err=%v want ErrUnknownMechanism", err)
	}
}

func TestDispatcher_PrepareAll(t *testing.T) {
	cedarH := &stubHandler{mechanism: "cedar"}
	d := NewDispatcher()
	d.Register(cedarH)

	entries := []Entry{
		{CartridgeRef: "a", Manifest: cartridges.Manifest{RuleID: "r1", Mechanism: "cedar"}},
		{CartridgeRef: "a", Manifest: cartridges.Manifest{RuleID: "r2", Mechanism: "cedar"}},
		// Unknown mechanism is silently skipped.
		{CartridgeRef: "a", Manifest: cartridges.Manifest{RuleID: "r3", Mechanism: "llm_classification"}},
	}
	ok, errs := d.PrepareAll(context.Background(), entries)
	if ok != 2 {
		t.Fatalf("ok=%d want 2", ok)
	}
	if len(errs) != 0 {
		t.Fatalf("errs=%v", errs)
	}
	if len(cedarH.prepared) != 2 {
		t.Fatalf("prepared=%v", cedarH.prepared)
	}
}
