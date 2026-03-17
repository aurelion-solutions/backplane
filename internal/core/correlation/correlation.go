// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package correlation carries the request-scoped correlation id across
// goroutines through context.Context. Mirrors kernel's
// src.core.context.correlation_id_var contract:
//
//   - HTTP middleware reads X-Correlation-ID or generates a fresh UUID.
//   - The value lives on the request context.
//   - Service code that needs to stamp an event / log line / RPC call
//     reads it via ID(ctx) and, if absent, falls back to Ensure(ctx).
//
// The package does NOT depend on echo, slog, or amqp — every transport
// integrates by reading / writing the context directly.
package correlation

import (
	"context"

	"github.com/google/uuid"
)

// HeaderName is the canonical request/response header that carries the
// correlation id. Same spelling as kernel's CorrelationIdMiddleware.
const HeaderName = "X-Correlation-ID"

type ctxKey int

const correlationIDKey ctxKey = iota

// WithID returns ctx annotated with id. Empty id is a no-op (returns
// the same ctx) so callers can safely funnel optional values through.
func WithID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, correlationIDKey, id)
}

// ID returns the correlation id attached to ctx, or "" if absent.
// The boolean reports presence so callers can distinguish "no header
// supplied" from "header was empty".
func ID(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	v, ok := ctx.Value(correlationIDKey).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// Ensure returns a context that is guaranteed to carry a correlation
// id, plus the id itself. If ctx already has one, it is reused; if
// not, a fresh UUID v4 is generated and attached.
//
// Use this at the boundary of any code path that must emit traceable
// signals but cannot guarantee an upstream caller set the id.
func Ensure(ctx context.Context) (context.Context, string) {
	if id, ok := ID(ctx); ok {
		return ctx, id
	}
	id := uuid.NewString()
	return WithID(ctx, id), id
}
