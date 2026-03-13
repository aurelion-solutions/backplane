// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package llm delivers streaming chat completions through pluggable
// backends — on-prem (llama.cpp / GGUF), Anthropic API, OpenAI API,
// future providers. One package holds the contract, the Factory
// registry, and every provider.
//
// Streaming uses a Go channel: Stream returns <-chan Chunk that is
// closed when the stream finishes (the last Chunk has Done=true).
// Cancellation is via the passed-in context — no separate Abort method.
package llm

import (
	"context"
	"errors"
)

// ErrNotImplemented is returned by stub providers whose backend
// integration has not been written yet.
var ErrNotImplemented = errors.New("llm: provider not implemented")

// Role is the message role in a chat-style exchange.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Valid reports whether the role matches a defined constant.
func (r Role) Valid() bool {
	switch r {
	case RoleSystem, RoleUser, RoleAssistant:
		return true
	}
	return false
}

// Message is one chat turn sent to a Provider. Immutable.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// Chunk is one streamed token (or the terminal record).
//
// While Done == false only Token is meaningful. The terminal Chunk
// carries Done == true and MUST populate Output (assembled text) and
// TokensUsed. Providers MUST emit exactly one terminal Chunk per
// stream — including on context cancellation.
type Chunk struct {
	Token      string `json:"token"`
	Done       bool   `json:"done"`
	Output     string `json:"output,omitempty"`
	TokensUsed int    `json:"tokens_used,omitempty"`
}

// Provider streams chat completions for one backend.
//
// Stream returns a receive-only channel of Chunks. The channel is
// closed after the terminal Chunk is sent. Cancel via ctx — the
// provider must emit a terminal Done=true Chunk before closing,
// even on cancellation.
type Provider interface {
	Stream(ctx context.Context, messages []Message, params map[string]any) (<-chan Chunk, error)
}
