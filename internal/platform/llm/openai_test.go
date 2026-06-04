// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sseServer fakes an OpenAI-compatible /chat/completions streaming
// endpoint that emits the given delta tokens then [DONE].
func sseServer(t *testing.T, tokens []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.Error(w, "wrong path", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl, _ := w.(http.Flusher)
		for _, tok := range tokens {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", tok)
			if fl != nil {
				fl.Flush()
			}
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
}

func drainOpenAI(t *testing.T, baseURL string) (string, int, bool) {
	t.Helper()
	c := &OpenAICompat{cfg: Config{BaseURL: baseURL, Model: "qwen2.5-7b-instruct"}, client: &http.Client{}}
	ch, err := c.Stream(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var got strings.Builder
	var terminal Chunk
	sawTerminal := false
	for chunk := range ch {
		if chunk.Done {
			terminal = chunk
			sawTerminal = true
			continue
		}
		got.WriteString(chunk.Token)
	}
	if got.String() != terminal.Output {
		t.Fatalf("assembled %q != terminal output %q", got.String(), terminal.Output)
	}
	return terminal.Output, terminal.TokensUsed, sawTerminal
}

func TestOpenAIStreamAssembles(t *testing.T) {
	srv := sseServer(t, []string{"Priv", "ileged", " account"})
	defer srv.Close()

	out, tokens, sawTerminal := drainOpenAI(t, srv.URL+"/v1")
	if !sawTerminal {
		t.Fatal("no terminal chunk")
	}
	if out != "Privileged account" {
		t.Fatalf("output = %q", out)
	}
	if tokens != 3 {
		t.Fatalf("tokens = %d, want 3", tokens)
	}
}

func TestOpenAIStreamErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "model not loaded", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := &OpenAICompat{cfg: Config{BaseURL: srv.URL + "/v1", Model: "m"}, client: &http.Client{}}
	_, err := c.Stream(context.Background(), []Message{{Role: RoleUser, Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("expected error on non-200 status, got nil")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Fatalf("error %q should mention status 503", err.Error())
	}
}

func TestOpenAIStreamCancellation(t *testing.T) {
	srv := sseServer(t, []string{"a", "b", "c", "d", "e"})
	defer srv.Close()

	c := &OpenAICompat{cfg: Config{BaseURL: srv.URL + "/v1", Model: "m"}, client: &http.Client{}}
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := c.Stream(ctx, []Message{{Role: RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	// Read one chunk, cancel, then drain — the goroutine must not block.
	<-ch
	cancel()
	for range ch { //nolint:revive // draining
	}
}
