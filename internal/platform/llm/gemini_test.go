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

// geminiSSE fakes the Gemini streamGenerateContent endpoint.
func geminiSSE(t *testing.T, capturePath *string, capture *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if capturePath != nil {
			*capturePath = r.URL.Path
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
		emit("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Priv\"}]}}]}\n\n")
		emit("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ileged\"}]}}],\"usageMetadata\":{\"candidatesTokenCount\":7}}\n\n")
	}))
}

func TestGeminiStreamAssembles(t *testing.T) {
	var path string
	var sent map[string]any
	srv := geminiSSE(t, &path, &sent)
	defer srv.Close()

	c := &Gemini{cfg: Config{BaseURL: srv.URL, Model: "gemini-2.0-flash", APIKey: "k"}, client: &http.Client{}}
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
		t.Fatalf("tokens = %d, want 7 (from usageMetadata)", terminal.TokensUsed)
	}
	// Model is in the URL path, not the body.
	if !strings.Contains(path, "gemini-2.0-flash:streamGenerateContent") {
		t.Fatalf("path = %q, want model:streamGenerateContent", path)
	}
	// system → systemInstruction; turns → contents with role user.
	if _, ok := sent["systemInstruction"]; !ok {
		t.Fatal("systemInstruction missing")
	}
	if contents, _ := sent["contents"].([]any); len(contents) != 1 {
		t.Fatalf("contents len = %d, want 1", len(contents))
	}
}

func TestGeminiErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad key", http.StatusForbidden)
	}))
	defer srv.Close()

	c := &Gemini{cfg: Config{BaseURL: srv.URL, Model: "m"}, client: &http.Client{}}
	_, err := c.Stream(context.Background(), []Message{{Role: RoleUser, Content: "x"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("err = %v, want one mentioning 403", err)
	}
}

func TestGeminiRoleMapping(t *testing.T) {
	var sent map[string]any
	srv := geminiSSE(t, nil, &sent)
	defer srv.Close()

	c := &Gemini{cfg: Config{BaseURL: srv.URL, Model: "m"}, client: &http.Client{}}
	ch, err := c.Stream(context.Background(), []Message{
		{Role: RoleUser, Content: "q"},
		{Role: RoleAssistant, Content: "a"},
	}, nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	for range ch { //nolint:revive // drain
	}
	contents, _ := sent["contents"].([]any)
	if len(contents) != 2 {
		t.Fatalf("contents = %d, want 2", len(contents))
	}
	second, _ := contents[1].(map[string]any)
	if second["role"] != "model" {
		t.Fatalf("assistant role mapped to %v, want model", second["role"])
	}
}
