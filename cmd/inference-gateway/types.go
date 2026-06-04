// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package main

import "github.com/aurelion-solutions/backplane/internal/platform/llm"

// StreamRequest is the body of POST /v1/inference/stream.
//
// Provider names a configured entry in config.LLM.Providers
// (e.g. "qwen-local", "claude"). Empty means the configured default.
// Messages and Params are passed straight through to the provider —
// the gateway defines no prompts of its own.
type StreamRequest struct {
	Provider string         `json:"provider,omitempty"`
	Messages []llm.Message  `json:"messages"`
	Params   map[string]any `json:"params,omitempty"`
}
