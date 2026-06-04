// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aurelion-solutions/backplane/internal/platform/llm"
	"github.com/labstack/echo/v4"
)

// fakeExecutor streams a fixed set of chunks, or fails up front.
type fakeExecutor struct {
	chunks []llm.Chunk
	err    error
}

func (f fakeExecutor) Stream(_ context.Context, _ StreamRequest) (<-chan llm.Chunk, error) {
	if f.err != nil {
		return nil, f.err
	}
	ch := make(chan llm.Chunk, len(f.chunks))
	for _, c := range f.chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func doStream(t *testing.T, exec Executor, body string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	registerInferenceRoutes(e, streamDeps{exec: exec, log: discardLog()})
	req := httptest.NewRequest(http.MethodPost, "/v1/inference/stream", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestStreamRelaysSSE(t *testing.T) {
	exec := fakeExecutor{chunks: []llm.Chunk{
		{Token: "Priv", Done: false},
		{Token: "ileged", Done: false},
		{Done: true, Output: "Privileged", TokensUsed: 2},
	}}
	rec := doStream(t, exec, `{"messages":[{"role":"user","content":"hi"}]}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get(echo.HeaderContentType); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}
	out := rec.Body.String()
	for _, want := range []string{
		`data: {"done":false,"token":"Priv"}`,
		`data: {"done":false,"token":"ileged"}`,
		`"done":true`,
		`"output":"Privileged"`,
		`"tokens_used":2`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("SSE body missing %q\n---\n%s", want, out)
		}
	}
}

func TestStreamNotImplementedIsJSON501(t *testing.T) {
	exec := fakeExecutor{err: llm.ErrNotImplemented}
	rec := doStream(t, exec, `{"messages":[{"role":"user","content":"hi"}]}`)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
	if ct := rec.Header().Get(echo.HeaderContentType); !strings.HasPrefix(ct, echo.MIMEApplicationJSON) {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
}

func TestStreamEmptyMessagesIs400(t *testing.T) {
	exec := fakeExecutor{chunks: []llm.Chunk{{Done: true}}}
	rec := doStream(t, exec, `{"messages":[]}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
