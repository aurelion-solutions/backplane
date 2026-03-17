// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package webserver

import (
	"strings"

	"github.com/aurelion-solutions/backplane/internal/core/correlation"
	"github.com/labstack/echo/v4"
)

// correlationIDMiddleware echoes or generates X-Correlation-ID for every
// HTTP request and attaches it to the request context so downstream
// handlers / services / RPC calls share one trace id.
//
// Pipeline per request:
//
//  1. Read X-Correlation-ID from request headers; strip whitespace;
//     treat empty as absent.
//  2. If absent — generate a fresh UUID v4 via correlation.Ensure.
//  3. Attach to request context via correlation.WithID so service-layer
//     callers can pick it up with correlation.ID(ctx).
//  4. Echo the value in the response header.
func correlationIDMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			raw := strings.TrimSpace(c.Request().Header.Get(correlation.HeaderName))
			ctx, id := correlation.Ensure(correlation.WithID(c.Request().Context(), raw))
			c.SetRequest(c.Request().WithContext(ctx))
			c.Response().Header().Set(correlation.HeaderName, id)
			return next(c)
		}
	}
}
