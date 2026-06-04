// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package finding_explanation

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// gatewayStub fakes cmd/inference-gateway's POST /v1/inference/stream,
// emitting token events then a terminal event in the gateway's SSE
// shape.
func gatewayStub(t *testing.T, tokens []string, output string, tokensUsed int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/inference/stream") {
			http.Error(w, "wrong path", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl, _ := w.(http.Flusher)
		for _, tok := range tokens {
			fmt.Fprintf(w, "data: {\"token\":%q,\"done\":false}\n\n", tok)
			if fl != nil {
				fl.Flush()
			}
		}
		fmt.Fprintf(w, "data: {\"output\":%q,\"tokens_used\":%d,\"done\":true}\n\n", output, tokensUsed)
	}))
}

func TestGatewayClientAssembles(t *testing.T) {
	srv := gatewayStub(t, []string{"Priv", "ileged"}, "Privileged account [F]", 9)
	defer srv.Close()

	c := NewGatewayClient(srv.URL, nil)
	res, err := c.Generate(context.Background(), GenerateRequest{
		Provider: "qwen-local",
		Messages: []InferenceMessage{{Role: "user", Content: "explain"}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if res.Output != "Privileged account [F]" {
		t.Fatalf("output = %q", res.Output)
	}
	if res.TokensUsed != 9 {
		t.Fatalf("tokens = %d, want 9", res.TokensUsed)
	}
	if res.ModelRef != "qwen-local" {
		t.Fatalf("model_ref = %q, want qwen-local", res.ModelRef)
	}
}

func TestGatewayClientErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "provider not implemented", http.StatusNotImplemented)
	}))
	defer srv.Close()

	c := NewGatewayClient(srv.URL, nil)
	_, err := c.Generate(context.Background(), GenerateRequest{
		Messages: []InferenceMessage{{Role: "user", Content: "x"}},
	})
	if err == nil {
		t.Fatal("expected error on 501, got nil")
	}
	if !strings.Contains(err.Error(), "501") {
		t.Fatalf("error %q should mention 501", err.Error())
	}
}

// The gateway client satisfies the engine's InferenceClient port.
var _ InferenceClient = (*GatewayClient)(nil)
