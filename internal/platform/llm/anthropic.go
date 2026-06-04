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

// anthropicVersion is the required Anthropic API version header value.
const anthropicVersion = "2023-06-01"

// defaultMaxTokens is the fallback when the caller does not set
// max_tokens — the Anthropic Messages API requires the field.
const defaultMaxTokens = 1024

// Anthropic is the Anthropic Messages API client (Claude family). It
// speaks Anthropic's own wire format — system is a top-level field, not
// a message, and the SSE event shape differs from OpenAI — so it is a
// distinct protocol from the OpenAI-compatible client.
type Anthropic struct {
	cfg    Config
	client *http.Client
}

// RegisterAnthropic wires the "anthropic" protocol client into f.
func RegisterAnthropic(f *Factory) {
	f.Register("anthropic", func(cfg Config) (Provider, error) {
		return &Anthropic{cfg: cfg, client: &http.Client{}}, nil
	})
}

// anthropicEvent is the slice of one SSE data line this client reads.
// Anthropic streams typed events; we care about text deltas and the
// final usage.
type anthropicEvent struct {
	Type  string `json:"type"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
	Usage *struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// Stream implements Provider against POST {base}/v1/messages with
// stream=true.
func (c *Anthropic) Stream(ctx context.Context, messages []Message, params map[string]any) (<-chan Chunk, error) {
	system, turns := splitSystem(messages)
	body, err := buildAnthropicBody(c.cfg.Model, system, turns, params)
	if err != nil {
		return nil, err
	}

	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/v1/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("anthropic-version", anthropicVersion)
	if c.cfg.APIKey != "" {
		req.Header.Set("x-api-key", c.cfg.APIKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("llm: anthropic backend status %d: %s", resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	ch := make(chan Chunk)
	go c.relay(ctx, resp.Body, ch)
	return ch, nil
}

// relay reads the Anthropic SSE body, forwards text deltas, and always
// emits a terminal chunk before closing the channel.
func (c *Anthropic) relay(ctx context.Context, body io.ReadCloser, ch chan<- Chunk) {
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
		var ev anthropicEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "content_block_delta":
			if ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
				assembled.WriteString(ev.Delta.Text)
				tokens++
				if !send(ctx, ch, Chunk{Token: ev.Delta.Text}) {
					return
				}
			}
		case "message_delta":
			if ev.Usage != nil && ev.Usage.OutputTokens > 0 {
				tokens = ev.Usage.OutputTokens
			}
		case "message_stop":
			send(ctx, ch, Chunk{Done: true, Output: assembled.String(), TokensUsed: tokens})
			return
		}
	}

	send(ctx, ch, Chunk{Done: true, Output: assembled.String(), TokensUsed: tokens})
}

// buildAnthropicBody marshals the Messages request. system is a
// top-level field; max_tokens is required and defaulted. Caller params
// (temperature, top_p, …) layer in but cannot override the fixed
// fields.
func buildAnthropicBody(model, system string, turns []Message, params map[string]any) ([]byte, error) {
	merged := map[string]any{}
	for k, v := range params {
		merged[k] = v
	}
	if _, ok := merged["max_tokens"]; !ok {
		merged["max_tokens"] = defaultMaxTokens
	}
	merged["model"] = model
	merged["messages"] = turns
	merged["stream"] = true
	if system != "" {
		merged["system"] = system
	}
	return json.Marshal(merged)
}

// splitSystem separates system messages (joined) from the user/assistant
// turns. Backends that carry system as a dedicated field (Anthropic,
// Gemini) use this instead of inlining it as a message.
func splitSystem(messages []Message) (system string, turns []Message) {
	var sys strings.Builder
	for _, m := range messages {
		if m.Role == RoleSystem {
			if sys.Len() > 0 {
				sys.WriteString("\n\n")
			}
			sys.WriteString(m.Content)
			continue
		}
		turns = append(turns, m)
	}
	return sys.String(), turns
}
