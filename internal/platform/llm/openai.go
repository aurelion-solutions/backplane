// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package llm

// OpenAI is a placeholder for the OpenAI Chat Completions API backend
// (GPT family).
type OpenAI struct{ Stub }

// RegisterOpenAI wires the "openai" stub provider.
func RegisterOpenAI(f *Factory) {
	f.Register("openai", func() (Provider, error) { return OpenAI{}, nil })
}
