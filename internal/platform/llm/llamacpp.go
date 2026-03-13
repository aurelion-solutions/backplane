// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package llm

// LlamaCpp is a placeholder for an on-prem GGUF model backend
// (llama.cpp / llama-cpp-go bindings).
type LlamaCpp struct{ Stub }

// RegisterLlamaCpp wires the "llamacpp" stub provider.
func RegisterLlamaCpp(f *Factory) {
	f.Register("llamacpp", func() (Provider, error) { return LlamaCpp{}, nil })
}
