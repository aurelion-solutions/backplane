// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package config

import (
	"testing"

	"github.com/aurelion-solutions/backplane/internal/platform/secretmanagers"
)

// fakeManager is a read-only Manager backed by a map. A missing key
// returns ErrNotFound so decodeOptional falls back to defaults.
type fakeManager map[string]string

func (m fakeManager) Get(key string) (string, error) {
	v, ok := m[key]
	if !ok {
		return "", secretmanagers.ErrNotFound
	}
	return v, nil
}

func TestLoadLLMDefaultsWhenAbsent(t *testing.T) {
	l, err := loadLLM(fakeManager{})
	if err != nil {
		t.Fatalf("loadLLM: %v", err)
	}
	if l.Provider != "qwen-local" {
		t.Fatalf("default provider = %q, want qwen-local", l.Provider)
	}
	active, err := l.Active()
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if active.Protocol != "openai" {
		t.Fatalf("default protocol = %q, want openai", active.Protocol)
	}
}

func TestLoadLLMFromSecret(t *testing.T) {
	raw := `{
		"provider": "claude",
		"providers": {
			"claude": {"protocol": "anthropic", "base_url": "https://api.anthropic.com", "api_key": "k", "model": "claude-sonnet-4-6"}
		}
	}`
	l, err := loadLLM(fakeManager{"llm": raw})
	if err != nil {
		t.Fatalf("loadLLM: %v", err)
	}
	active, err := l.Active()
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if active.Protocol != "anthropic" || active.Model != "claude-sonnet-4-6" {
		t.Fatalf("active = %+v, want anthropic/claude-sonnet-4-6", active)
	}
}

func TestLLMActiveMissingProvider(t *testing.T) {
	l := LLM{Provider: "ghost", Providers: map[string]LLMProvider{}}
	if _, err := l.Active(); err == nil {
		t.Fatal("expected error for missing active provider, got nil")
	}
}
