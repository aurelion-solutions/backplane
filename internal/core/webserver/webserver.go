// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package webserver constructs the echo HTTP server with the
// bootstrap middleware stack (recover, request-id, CORS, slog access
// log) and exposes a single /healthz endpoint.
//
// This package depends on nothing inside backplane (no config, no
// domain). The caller composes a Config and passes it in. Domain
// routes are wired by callers via the returned *echo.Echo.
package webserver

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Config holds the inputs New needs.
type Config struct {
	Debug            bool
	CORSAllowOrigins []string
}

// New returns a configured echo.Echo. The caller mounts domain routes
// and runs (*echo.Echo).Start. Shutdown is the caller's responsibility.
func New(cfg Config, log *slog.Logger) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Debug = cfg.Debug

	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: cfg.CORSAllowOrigins,
	}))
	e.Use(slogRequestLogger(log))

	e.GET("/healthz", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	return e
}

func slogRequestLogger(log *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			err := next(c)
			req := c.Request()
			res := c.Response()
			log.Info("http",
				slog.String("method", req.Method),
				slog.String("path", req.URL.Path),
				slog.Int("status", res.Status),
				slog.String("request_id", res.Header().Get(echo.HeaderXRequestID)),
			)
			return err
		}
	}
}
