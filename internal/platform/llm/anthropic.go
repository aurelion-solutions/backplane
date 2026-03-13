// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package llm

// Anthropic is a placeholder for the Anthropic Messages API backend
// (Claude family).
type Anthropic struct{ Stub }

// RegisterAnthropic wires the "anthropic" stub provider.
func RegisterAnthropic(f *Factory) {
	f.Register("anthropic", func() (Provider, error) { return Anthropic{}, nil })
}
