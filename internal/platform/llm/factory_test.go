// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package llm

import (
	"context"
	"errors"
	"testing"
)

// registerAll wires every shipped protocol client, mirroring the
// composition root.
func registerAll(f *Factory) {
	RegisterOpenAI(f)
	RegisterAnthropic(f)
	RegisterGemini(f)
}

func TestFactoryGetByProtocol(t *testing.T) {
	f := NewFactory()
	registerAll(f)

	for _, proto := range []string{"openai", "anthropic", "gemini"} {
		p, err := f.Get(proto, Config{BaseURL: "http://x", Model: "m"})
		if err != nil {
			t.Fatalf("Get(%q): unexpected error: %v", proto, err)
		}
		if p == nil {
			t.Fatalf("Get(%q): nil provider", proto)
		}
	}
}

func TestFactoryUnknownProtocol(t *testing.T) {
	f := NewFactory()
	registerAll(f)

	if _, err := f.Get("llamacpp", Config{}); err == nil {
		t.Fatal("expected error for unregistered protocol, got nil")
	}
}

func TestFactoryNamesSorted(t *testing.T) {
	f := NewFactory()
	registerAll(f)

	got := f.Names()
	want := []string{"anthropic", "gemini", "openai"}
	if len(got) != len(want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names() = %v, want %v", got, want)
		}
	}
}

func TestStubTypeNotImplemented(t *testing.T) {
	// All shipped protocol clients are implemented; Stub remains the
	// embed-able no-op for a future not-yet-written protocol.
	var s Stub
	_, err := s.Stream(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil)
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Stub.Stream err = %v, want ErrNotImplemented", err)
	}
}
