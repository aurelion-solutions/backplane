// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package finding_explanation

import "testing"

func refsFixture() []Reference {
	return []Reference{
		{Label: "F", Kind: RefFinding, ID: "f-1"},
		{Label: "P1", Kind: RefPolicy, ID: "p-1"},
		{Label: "E1", Kind: RefEvidence, ID: "e-1"},
	}
}

func TestValidateCitationsKeepsOnlyKnownLabels(t *testing.T) {
	narrative := "The account [F] violates policy [P1], proven by [E1]. Also [E9] is invented."
	cited, stray := validateCitations(narrative, refsFixture())

	if len(cited) != 3 {
		t.Fatalf("cited = %d, want 3 (%+v)", len(cited), cited)
	}
	if len(stray) != 1 || stray[0] != "E9" {
		t.Fatalf("stray = %v, want [E9]", stray)
	}
}

func TestValidateCitationsDeduplicates(t *testing.T) {
	narrative := "[F] and again [F] and [E1] [E1]."
	cited, _ := validateCitations(narrative, refsFixture())
	if len(cited) != 2 {
		t.Fatalf("cited = %d, want 2 (dedup)", len(cited))
	}
}

func TestValidateCitationsNone(t *testing.T) {
	cited, stray := validateCitations("no citations here", refsFixture())
	if len(cited) != 0 || len(stray) != 0 {
		t.Fatalf("cited=%v stray=%v, want both empty", cited, stray)
	}
}

func TestValidateCitationsCarriesKindAndID(t *testing.T) {
	cited, _ := validateCitations("[P1]", refsFixture())
	if len(cited) != 1 || cited[0].Kind != RefPolicy || cited[0].ID != "p-1" {
		t.Fatalf("citation = %+v, want policy/p-1", cited)
	}
}
