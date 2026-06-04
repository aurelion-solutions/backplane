// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"context"
	"testing"

	"github.com/aurelion-solutions/backplane/internal/core/config"
	"github.com/aurelion-solutions/backplane/internal/platform/llm"
)

func testConfig() config.LLM {
	return config.LLM{
		Provider: "qwen-local",
		Providers: map[string]config.LLMProvider{
			"qwen-local": {Protocol: "openai", BaseURL: "http://localhost:8080/v1", Model: "qwen2.5-7b-instruct"},
			// Unreachable base so a Stream call fails on transport without
			// touching the network — proves the executor reaches the provider.
			"claude": {Protocol: "anthropic", BaseURL: "http://127.0.0.1:1", APIKey: "k", Model: "claude-sonnet-4-6"},
		},
	}
}

func newExecutor() *LocalExecutor {
	llf := llm.NewFactory()
	llm.RegisterOpenAI(llf)
	llm.RegisterAnthropic(llf)
	llm.RegisterGemini(llf)
	return NewLocalExecutor(llf, testConfig())
}

func TestLocalExecutorResolvesDefault(t *testing.T) {
	e := newExecutor()
	got, err := e.resolve("")
	if err != nil {
		t.Fatalf("resolve(default): %v", err)
	}
	if got.Protocol != "openai" {
		t.Fatalf("default protocol = %q, want openai", got.Protocol)
	}
}

func TestLocalExecutorResolvesNamed(t *testing.T) {
	e := newExecutor()
	got, err := e.resolve("claude")
	if err != nil {
		t.Fatalf("resolve(claude): %v", err)
	}
	if got.Protocol != "anthropic" {
		t.Fatalf("claude protocol = %q, want anthropic", got.Protocol)
	}
}

func TestLocalExecutorUnknownProvider(t *testing.T) {
	e := newExecutor()
	if _, err := e.resolve("ghost"); err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
}

func TestLocalExecutorStreamReachesProvider(t *testing.T) {
	e := newExecutor()
	// "claude" resolves to the anthropic protocol pointed at an
	// unreachable base. The executor must wire through to the provider
	// (resolution + factory + provider) and surface its transport error,
	// rather than failing earlier.
	_, err := e.Stream(context.Background(), StreamRequest{
		Provider: "claude",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected a transport error reaching the provider, got nil")
	}
}
