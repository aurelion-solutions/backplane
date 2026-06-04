// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package finding_explanation

import "testing"

func ctxFixture() explanationContext {
	return explanationContext{
		refs: []Reference{
			{Label: "F", Kind: RefFinding, ID: "f-1", Detail: "privileged_access severity=high"},
			{Label: "E1", Kind: RefEvidence, ID: "e-1", Detail: "policy_ref=x chain_hash=abc"},
		},
	}
}

func TestInputHashDeterministic(t *testing.T) {
	c := ctxFixture()
	if inputHash(c, "qwen-local", "fr") != inputHash(c, "qwen-local", "fr") {
		t.Fatal("inputHash not deterministic for identical inputs")
	}
}

func TestInputHashSensitiveToProvider(t *testing.T) {
	c := ctxFixture()
	if inputHash(c, "qwen-local", "") == inputHash(c, "claude", "") {
		t.Fatal("inputHash should differ by provider")
	}
}

func TestInputHashSensitiveToRefs(t *testing.T) {
	a := ctxFixture()
	b := ctxFixture()
	b.refs[0].Detail = "privileged_access severity=critical" // severity changed
	if inputHash(a, "qwen-local", "") == inputHash(b, "qwen-local", "") {
		t.Fatal("inputHash should change when a ref detail changes")
	}
}

func TestInputHashSensitiveToLanguage(t *testing.T) {
	c := ctxFixture()
	if inputHash(c, "qwen-local", "fr") == inputHash(c, "qwen-local", "ru") {
		t.Fatal("inputHash should differ by requested language")
	}
	// Unknown / empty collapse to the same default key.
	if inputHash(c, "qwen-local", "") != inputHash(c, "qwen-local", "klingon") {
		t.Fatal("unknown language should hash as the default (English)")
	}
	// Normalisation: case/whitespace must not split the cache.
	if inputHash(c, "qwen-local", "fr") != inputHash(c, "qwen-local", " FR ") {
		t.Fatal("language should be normalised before hashing")
	}
}

func TestRenderMessagesCarriesSystemAndLabels(t *testing.T) {
	msgs := renderMessages(ctxFixture(), "")
	if len(msgs) != 2 || msgs[0].Role != "system" || msgs[1].Role != "user" {
		t.Fatalf("messages shape = %+v", msgs)
	}
	if !contains(msgs[1].Content, "[F]") || !contains(msgs[1].Content, "[E1]") {
		t.Fatalf("user message missing labels:\n%s", msgs[1].Content)
	}
}

func TestRenderMessagesAppliesLanguageDirective(t *testing.T) {
	fr := renderMessages(ctxFixture(), "fr")
	if !contains(fr[0].Content, "French") {
		t.Fatalf("system prompt missing French directive:\n%s", fr[0].Content)
	}
	// Default and unknown languages add no directive.
	def := renderMessages(ctxFixture(), "")
	if contains(def[0].Content, "Write the explanation in") {
		t.Fatalf("default should carry no language directive:\n%s", def[0].Content)
	}
	if renderMessages(ctxFixture(), "klingon")[0].Content != def[0].Content {
		t.Fatal("unknown language should fall back to the default prompt")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
