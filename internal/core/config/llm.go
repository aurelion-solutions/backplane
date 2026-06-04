// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package config

import (
	"fmt"

	"github.com/aurelion-solutions/backplane/internal/platform/secretmanagers"
)

// LLM selects and configures chat-completion backends.
//
// Taxonomy: a backend is sorted by its wire protocol, not its brand.
// One protocol is one client implementation; a brand is just a named
// entry pointing at a protocol plus its endpoint coordinates. The
// OpenAI-compatible protocol covers the local llama-server (our Qwen),
// OpenAI, DeepSeek, Mistral and most others — they differ only by
// base_url / api_key / model. Anthropic and Gemini speak their own
// protocols and get their own clients.
type LLM struct {
	// Provider is the active named entry in Providers used by default.
	Provider string `json:"provider"`
	// Providers maps a chosen name (e.g. "qwen-local", "openai",
	// "deepseek", "claude", "gemini") to its protocol + endpoint.
	Providers map[string]LLMProvider `json:"providers"`
	// GatewayURL is where callers (backplane, worker, pdp) reach the
	// inference-gateway process. The gateway itself reads Provider /
	// Providers; everyone else reads this.
	GatewayURL string `json:"gateway_url"`
}

// LLMProvider is one named backend entry.
type LLMProvider struct {
	// Protocol is the wire format — "openai" (OpenAI-compatible),
	// "anthropic", or "gemini". It selects the client implementation.
	Protocol string `json:"protocol"`
	// BaseURL is the endpoint root. For the local llama-server this is
	// the on-prem host; for SaaS it is the vendor API root.
	BaseURL string `json:"base_url"`
	// APIKey authenticates SaaS calls. Empty for a local llama-server.
	APIKey string `json:"api_key"`
	// Model is the model identifier passed in each request.
	Model string `json:"model"`
}

// DefaultLLM points at a local llama-server speaking the
// OpenAI-compatible protocol — the on-prem path where client data never
// leaves the perimeter.
func DefaultLLM() LLM {
	return LLM{
		Provider: "qwen-local",
		Providers: map[string]LLMProvider{
			"qwen-local": {
				Protocol: "openai",
				BaseURL:  "http://localhost:8080/v1",
				APIKey:   "",
				Model:    "qwen2.5-7b-instruct",
			},
		},
		GatewayURL: "http://localhost:8090",
	}
}

// Active returns the configured default provider entry.
func (l LLM) Active() (LLMProvider, error) {
	p, ok := l.Providers[l.Provider]
	if !ok {
		return LLMProvider{}, fmt.Errorf("config: llm active provider %q not found in providers", l.Provider)
	}
	return p, nil
}

func loadLLM(sm secretmanagers.Manager) (LLM, error) {
	l := DefaultLLM()
	if err := decodeOptional(sm, "llm", &l); err != nil {
		return LLM{}, err
	}
	return l, nil
}
