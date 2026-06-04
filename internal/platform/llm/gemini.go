// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package llm

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

// Gemini is the Google Gemini API client. Gemini speaks its own wire
// format — contents with parts, roles user/model, a separate
// systemInstruction — so it is a distinct protocol from the
// OpenAI-compatible and Anthropic clients.
type Gemini struct {
	cfg    Config
	client *http.Client
}

// RegisterGemini wires the "gemini" protocol client into f.
func RegisterGemini(f *Factory) {
	f.Register("gemini", func(cfg Config) (Provider, error) {
		return &Gemini{cfg: cfg, client: &http.Client{}}, nil
	})
}

// geminiPart / geminiContent shape the request body.
type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

// geminiEvent is the slice of one SSE data line this client reads.
type geminiEvent struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata *struct {
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
}

// Stream implements Provider against
// POST {base}/v1beta/models/{model}:streamGenerateContent?alt=sse.
func (c *Gemini) Stream(ctx context.Context, messages []Message, params map[string]any) (<-chan Chunk, error) {
	system, turns := splitSystem(messages)
	body, err := buildGeminiBody(system, turns, params)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse",
		strings.TrimRight(c.cfg.BaseURL, "/"), c.cfg.Model)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if c.cfg.APIKey != "" {
		req.Header.Set("x-goog-api-key", c.cfg.APIKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("llm: gemini backend status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	ch := make(chan Chunk)
	go c.relay(ctx, resp.Body, ch)
	return ch, nil
}

// relay reads the Gemini SSE body, forwards text parts, and always
// emits a terminal chunk before closing the channel.
func (c *Gemini) relay(ctx context.Context, body io.ReadCloser, ch chan<- Chunk) {
	defer close(ch)
	defer func() { _ = body.Close() }()

	var assembled strings.Builder
	tokens := 0

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		data, ok := strings.CutPrefix(strings.TrimSpace(scanner.Text()), "data:")
		if !ok {
			continue
		}
		var ev geminiEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &ev); err != nil {
			continue
		}
		if ev.UsageMetadata != nil && ev.UsageMetadata.CandidatesTokenCount > 0 {
			tokens = ev.UsageMetadata.CandidatesTokenCount
		}
		for _, cand := range ev.Candidates {
			for _, part := range cand.Content.Parts {
				if part.Text == "" {
					continue
				}
				assembled.WriteString(part.Text)
				if ev.UsageMetadata == nil {
					tokens++
				}
				if !send(ctx, ch, Chunk{Token: part.Text}) {
					return
				}
			}
		}
	}

	send(ctx, ch, Chunk{Done: true, Output: assembled.String(), TokensUsed: tokens})
}

// buildGeminiBody marshals the request: turns become contents (roles
// mapped user→user, assistant→model), system becomes systemInstruction,
// and pass-through params land under generationConfig.
func buildGeminiBody(system string, turns []Message, params map[string]any) ([]byte, error) {
	contents := make([]geminiContent, 0, len(turns))
	for _, m := range turns {
		role := "user"
		if m.Role == RoleAssistant {
			role = "model"
		}
		contents = append(contents, geminiContent{Role: role, Parts: []geminiPart{{Text: m.Content}}})
	}

	payload := map[string]any{"contents": contents}
	if system != "" {
		payload["systemInstruction"] = geminiContent{Parts: []geminiPart{{Text: system}}}
	}
	if len(params) > 0 {
		payload["generationConfig"] = params
	}
	return json.Marshal(payload)
}
