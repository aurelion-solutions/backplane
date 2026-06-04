// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package finding_explanation

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// GatewayClient is the production InferenceClient: an HTTP+SSE client to
// cmd/inference-gateway. It assembles the gateway's token stream into
// one result, so the engine works with whole text while the transport
// stays streaming. It is the only thing here that knows the gateway
// exists.
type GatewayClient struct {
	baseURL string
	client  *http.Client
}

// NewGatewayClient builds a GatewayClient for the gateway at baseURL
// (e.g. http://localhost:8090).
func NewGatewayClient(baseURL string, client *http.Client) *GatewayClient {
	if client == nil {
		client = &http.Client{}
	}
	return &GatewayClient{baseURL: strings.TrimRight(baseURL, "/"), client: client}
}

// gatewayEvent is one SSE data line from POST /v1/inference/stream.
type gatewayEvent struct {
	Token      string `json:"token"`
	Output     string `json:"output"`
	TokensUsed int    `json:"tokens_used"`
	Done       bool   `json:"done"`
}

// Generate implements InferenceClient. It posts the request to the
// gateway and drains the SSE stream into a single assembled result.
func (g *GatewayClient) Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
	body, err := json.Marshal(map[string]any{
		"provider": req.Provider,
		"messages": req.Messages,
		"params":   req.Params,
	})
	if err != nil {
		return GenerateResult{}, err
	}

	url := g.baseURL + "/v1/inference/stream"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return GenerateResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return GenerateResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return GenerateResult{}, fmt.Errorf("inference gateway status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	var assembled strings.Builder
	result := GenerateResult{ModelRef: modelRefFor(req.Provider)}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		data, ok := strings.CutPrefix(strings.TrimSpace(scanner.Text()), "data:")
		if !ok {
			continue
		}
		var ev gatewayEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &ev); err != nil {
			continue
		}
		if ev.Done {
			// Prefer the gateway's assembled output; fall back to the
			// tokens we accumulated.
			if ev.Output != "" {
				result.Output = ev.Output
			} else {
				result.Output = assembled.String()
			}
			result.TokensUsed = ev.TokensUsed
			return result, nil
		}
		assembled.WriteString(ev.Token)
	}
	if err := scanner.Err(); err != nil {
		return GenerateResult{}, err
	}
	// Stream ended without a terminal event — return what we have.
	result.Output = assembled.String()
	return result, nil
}

// modelRefFor records which configured backend produced the text. The
// gateway does not report the exact model in the stream, so we record
// the provider name (or "default") as provenance.
func modelRefFor(provider string) string {
	if provider == "" {
		return "default"
	}
	return provider
}
