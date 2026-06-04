// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package finding_explanation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// promptTemplateVersion is part of the cache key — bump it whenever the
// prompt wording changes so old explanations invalidate.
const promptTemplateVersion = "fe-v1"

// systemPrompt fixes the deterministic boundary in the model's
// instructions: explain only the supplied facts, cite every claim by
// its label, invent nothing.
const systemPrompt = `You are an identity-governance analyst. You explain an already-proven finding in plain language for a human operator.

Strict rules:
- Use ONLY the facts provided below. Do not introduce any account, policy, severity, risk, or remediation that is not in the provided references.
- Every claim you make must cite the label of the reference it rests on, in square brackets, e.g. [F], [P1], [E1].
- Do not invent labels. Cite only labels that appear in the references.
- You are explaining the finding, not deciding it. Do not propose a new severity or a new finding.
- Be concise: 2-4 sentences.`

// languageNames maps a short language code to the English name the model
// is instructed to write in. The whitelist doubles as input sanitation:
// only known codes ever reach the prompt body, so the language field
// cannot be used to inject arbitrary instructions.
var languageNames = map[string]string{
	"en": "English",
	"ru": "Russian",
	"fr": "French",
	"de": "German",
	"es": "Spanish",
	"it": "Italian",
	"pt": "Portuguese",
	"nl": "Dutch",
	"pl": "Polish",
	"uk": "Ukrainian",
	"zh": "Chinese",
	"ja": "Japanese",
}

// resolveLang normalises a requested language to a whitelisted code, or
// "" when it is empty/unknown (treated as the default English).
func resolveLang(lang string) string {
	l := strings.ToLower(strings.TrimSpace(lang))
	if _, ok := languageNames[l]; ok {
		return l
	}
	return ""
}

// languageDirective is the system-prompt line requesting the narrative in
// a specific language. Empty for the default (English), keeping the
// deterministic boundary intact — it only affects wording, never the
// cited facts.
func languageDirective(lang string) string {
	l := resolveLang(lang)
	if l == "" {
		return ""
	}
	return "\n- Write the explanation in " + languageNames[l] + "."
}

// renderMessages builds the system+user message pair for one
// explanation context, in the requested language.
func renderMessages(c explanationContext, lang string) []InferenceMessage {
	var b strings.Builder
	b.WriteString("Explain this finding.\n\nReferences:\n")
	for _, r := range c.refs {
		fmt.Fprintf(&b, "[%s] (%s) %s\n", r.Label, r.Kind, r.Detail)
	}
	return []InferenceMessage{
		{Role: "system", Content: systemPrompt + languageDirective(lang)},
		{Role: "user", Content: b.String()},
	}
}

// inputHash digests the prompt inputs into the cache key. It is computed
// before generation, so it keys on the configured provider name (stable
// in config) rather than the exact model_ref the gateway reports back.
// Any change in template version, provider, requested language, or the
// labelled facts yields a new hash and invalidates a cached explanation.
// Deterministic: refs are already in a stable order.
func inputHash(c explanationContext, providerKey, lang string) string {
	if providerKey == "" {
		providerKey = "default"
	}
	h := sha256.New()
	fmt.Fprintf(h, "v=%s\x00provider=%s\x00lang=%s\x00", promptTemplateVersion, providerKey, resolveLang(lang))
	for _, r := range c.refs {
		fmt.Fprintf(h, "%s\x1f%s\x1f%s\x1f%s\x1e", r.Label, r.Kind, r.ID, r.Detail)
	}
	return hex.EncodeToString(h.Sum(nil))
}
