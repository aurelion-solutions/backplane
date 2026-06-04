// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/aurelion-solutions/backplane/internal/platform/llm"
	"github.com/labstack/echo/v4"
)

// streamDeps carries what the stream handler needs.
type streamDeps struct {
	exec Executor
	log  *slog.Logger
}

// registerInferenceRoutes mounts POST /v1/inference/stream.
func registerInferenceRoutes(e *echo.Echo, deps streamDeps) {
	e.POST("/v1/inference/stream", deps.handleStream)
}

// handleStream binds the request, opens a provider stream, and relays
// chunks to the client as Server-Sent Events.
//
// Event payloads (one JSON object per `data:` line):
//   - token: {"token": "...", "done": false}
//   - final: {"output": "...", "tokens_used": N, "done": true}
//
// A backend that has not been wired yet fails before the stream starts
// and is reported as a plain JSON error with an HTTP status — no SSE is
// opened in that case.
func (d streamDeps) handleStream(c echo.Context) error {
	var req StreamRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if len(req.Messages) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "messages is required"})
	}

	ctx := c.Request().Context()
	ch, err := d.exec.Stream(ctx, req)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, llm.ErrNotImplemented) {
			status = http.StatusNotImplemented
		}
		return c.JSON(status, map[string]string{"error": err.Error()})
	}

	h := c.Response().Header()
	h.Set(echo.HeaderContentType, "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	c.Response().WriteHeader(http.StatusOK)
	c.Response().Flush()

	for chunk := range ch {
		var payload map[string]any
		if chunk.Done {
			payload = map[string]any{"output": chunk.Output, "tokens_used": chunk.TokensUsed, "done": true}
		} else {
			payload = map[string]any{"token": chunk.Token, "done": false}
		}
		b, mErr := json.Marshal(payload)
		if mErr != nil {
			d.log.Warn("inference stream: marshal chunk", slog.Any("err", mErr))
			continue
		}
		if _, wErr := fmt.Fprintf(c.Response(), "data: %s\n\n", b); wErr != nil {
			// Client went away mid-stream. ctx cancellation makes the
			// provider emit its terminal chunk and close ch; draining
			// the rest is cheap, so just stop writing.
			d.log.Debug("inference stream: client write failed", slog.Any("err", wErr))
			break
		}
		c.Response().Flush()
	}
	return nil
}
