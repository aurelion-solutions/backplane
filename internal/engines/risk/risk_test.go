// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package risk

import "testing"

func TestScore_FactorDecomposition(t *testing.T) {
	// A terminated, privileged, MFA-less, inactive account: every
	// amplifier fires and the total caps at 100.
	got := Score(Input{
		Severity:   "critical",
		Kind:       "terminated_access",
		Privileged: true,
		MFAEnabled: false,
		Active:     false,
	})
	// 40 + 20 + 15 + 18 + 20 = 113 → capped to 100.
	if got.Score != 100 {
		t.Fatalf("expected capped score 100, got %d", got.Score)
	}
	want := map[string]int{
		"severity:critical":            40,
		"privileged_account":           20,
		"privileged_without_mfa":       15,
		"inactive_account_with_access": 18,
		"terminated_subject":           20,
	}
	if len(got.Factors) != len(want) {
		t.Fatalf("expected %d factors, got %d: %+v", len(want), len(got.Factors), got.Factors)
	}
	for _, f := range got.Factors {
		if want[f.Name] != f.Points {
			t.Errorf("factor %q: want %d, got %d", f.Name, want[f.Name], f.Points)
		}
	}
}

func TestScore_LowSeverityCleanAccount(t *testing.T) {
	got := Score(Input{Severity: "low", Kind: "unused_access", Privileged: false, MFAEnabled: true, Active: true})
	if got.Score != 5 {
		t.Fatalf("expected score 5 (severity:low only), got %d (%+v)", got.Score, got.Factors)
	}
	if len(got.Factors) != 1 || got.Factors[0].Name != "severity:low" {
		t.Fatalf("expected single severity:low factor, got %+v", got.Factors)
	}
}

func TestScore_Deterministic(t *testing.T) {
	in := Input{Severity: "high", Kind: "privileged_access", Privileged: true, MFAEnabled: true, Active: true}
	a := Score(in)
	b := Score(in)
	if a.Score != b.Score || len(a.Factors) != len(b.Factors) {
		t.Fatalf("scoring not deterministic: %+v vs %+v", a, b)
	}
	// high(25) + privileged(20) = 45
	if a.Score != 45 {
		t.Fatalf("expected 45, got %d", a.Score)
	}
}

func TestScore_UnknownSeverityDefaultsLow(t *testing.T) {
	got := Score(Input{Severity: "", Kind: "", Privileged: false, MFAEnabled: true, Active: true})
	if got.Score != 5 {
		t.Fatalf("empty severity should default to low(5), got %d", got.Score)
	}
}
