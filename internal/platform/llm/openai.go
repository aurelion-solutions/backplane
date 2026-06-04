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

// OpenAICompat is the OpenAI-compatible chat-completions client. One
// implementation serves every backend that speaks the OpenAI wire
// format — the local llama-server (our Qwen), OpenAI, DeepSeek, Mistral
// and others. They are distinguished only by Config (base URL, key,
// model), never by code.
type OpenAICompat struct {
	cfg    Config
	client *http.Client
}

// RegisterOpenAI wires the "openai" protocol client into f.
func RegisterOpenAI(f *Factory) {
	f.Register("openai", func(cfg Config) (Provider, error) {
		return &OpenAICompat{cfg: cfg, client: &http.Client{}}, nil
	})
}

// streamChunk is the slice of one SSE chunk this client reads.
type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

// Stream implements Provider against an OpenAI-compatible
// /chat/completions endpoint with stream=true (SSE). It emits one
// non-terminal Chunk per delta token and exactly one terminal Chunk,
// and closes the channel — including on context cancellation.
func (c *OpenAICompat) Stream(ctx context.Context, messages []Message, params map[string]any) (<-chan Chunk, error) {
	body, err := buildBody(c.cfg.Model, messages, params)
	if err != nil {
		return nil, err
	}

	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("llm: openai backend status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	ch := make(chan Chunk)
	go c.relay(ctx, resp.Body, ch)
	return ch, nil
}

// relay reads the SSE body, forwards token chunks, and always emits a
// terminal chunk before closing the channel.
func (c *OpenAICompat) relay(ctx context.Context, body io.ReadCloser, ch chan<- Chunk) {
	defer close(ch)
	defer func() { _ = body.Close() }()

	var assembled strings.Builder
	tokens := 0

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			break
		}
		var sc streamChunk
		if err := json.Unmarshal([]byte(data), &sc); err != nil {
			continue
		}
		if sc.Usage != nil && sc.Usage.CompletionTokens > 0 {
			tokens = sc.Usage.CompletionTokens
		}
		for _, choice := range sc.Choices {
			if choice.Delta.Content == "" {
				continue
			}
			assembled.WriteString(choice.Delta.Content)
			if sc.Usage == nil {
				tokens++
			}
			if !send(ctx, ch, Chunk{Token: choice.Delta.Content}) {
				return
			}
		}
	}

	send(ctx, ch, Chunk{Done: true, Output: assembled.String(), TokensUsed: tokens})
}

// send delivers one chunk unless the context is cancelled first.
func send(ctx context.Context, ch chan<- Chunk, c Chunk) bool {
	select {
	case ch <- c:
		return true
	case <-ctx.Done():
		return false
	}
}

// buildBody marshals the chat request, layering pass-through params
// (temperature, max_tokens, …) under the fixed model/messages/stream
// fields. Caller params cannot override model/messages/stream.
func buildBody(model string, messages []Message, params map[string]any) ([]byte, error) {
	merged := map[string]any{}
	for k, v := range params {
		merged[k] = v
	}
	merged["model"] = model
	merged["messages"] = messages
	merged["stream"] = true
	return json.Marshal(merged)
}
