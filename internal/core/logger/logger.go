// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package logger constructs the process-wide slog.Logger.
//
// This package depends on nothing inside backplane and reads no env
// or config of its own. The caller chooses the writer and level.
package logger

import (
	"io"
	"log/slog"
	"strings"
)

// New returns a JSON slog.Logger writing to w at the given level.
// Empty level falls back to LevelInfo. Unknown level strings also
// fall back to LevelInfo — the caller is responsible for validation
// if stricter behaviour is needed.
func New(w io.Writer, level string) *slog.Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: parseLevel(level)})
	return slog.New(h)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
