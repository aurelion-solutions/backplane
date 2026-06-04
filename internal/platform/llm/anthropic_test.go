// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// anthropicSSE fakes the Anthropic Messages streaming endpoint: text
// deltas, a usage-bearing message_delta, then message_stop.
func anthropicSSE(t *testing.T, capture *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/messages") {
			http.Error(w, "wrong path", http.StatusNotFound)
			return
		}
		if capture != nil {
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, capture)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl, _ := w.(http.Flusher)
		emit := func(s string) {
			io.WriteString(w, s)
			if fl != nil {
				fl.Flush()
			}
		}
		emit("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Priv\"}}\n\n")
		emit("data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"ileged\"}}\n\n")
		emit("data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":7}}\n\n")
		emit("data: {\"type\":\"message_stop\"}\n\n")
	}))
}

func TestAnthropicStreamAssembles(t *testing.T) {
	var sent map[string]any
	srv := anthropicSSE(t, &sent)
	defer srv.Close()

	c := &Anthropic{cfg: Config{BaseURL: srv.URL, Model: "claude-sonnet-4-6", APIKey: "k"}, client: &http.Client{}}
	ch, err := c.Stream(context.Background(), []Message{
		{Role: RoleSystem, Content: "be terse"},
		{Role: RoleUser, Content: "hi"},
	}, nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var out strings.Builder
	var terminal Chunk
	for chunk := range ch {
		if chunk.Done {
			terminal = chunk
			continue
		}
		out.WriteString(chunk.Token)
	}
	if out.String() != "Privileged" || terminal.Output != "Privileged" {
		t.Fatalf("assembled %q / terminal %q, want Privileged", out.String(), terminal.Output)
	}
	if terminal.TokensUsed != 7 {
		t.Fatalf("tokens = %d, want 7 (from usage)", terminal.TokensUsed)
	}
	// system is a top-level field, not a message; messages carry only the turn.
	if sent["system"] != "be terse" {
		t.Fatalf("system field = %v, want 'be terse'", sent["system"])
	}
	if msgs, _ := sent["messages"].([]any); len(msgs) != 1 {
		t.Fatalf("messages len = %d, want 1 (system excluded)", len(msgs))
	}
	if _, ok := sent["max_tokens"]; !ok {
		t.Fatal("max_tokens must be present (Anthropic requires it)")
	}
}

func TestAnthropicErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "overloaded", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := &Anthropic{cfg: Config{BaseURL: srv.URL, Model: "m"}, client: &http.Client{}}
	_, err := c.Stream(context.Background(), []Message{{Role: RoleUser, Content: "x"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("err = %v, want one mentioning 429", err)
	}
}
