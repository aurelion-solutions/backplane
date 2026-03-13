// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package siem

import (
	"context"
	"errors"
)

// ErrNotImplemented is returned by stub Sink providers whose backend
// integration has not been written yet. Callers may distinguish
// "registered but unfinished" from a transport error via errors.Is.
var ErrNotImplemented = errors.New("logsink: provider not implemented")

// Sink delivers an Event to a backend (file, MQ, SIEM, …).
// Implementations must be safe for concurrent use.
type Sink interface {
	Emit(ctx context.Context, event Event) error
}

// Reader fetches recently emitted Events back from a backend that
// supports it (file, db). Most SIEM providers are write-only and
// won't implement this.
type Reader interface {
	Read(ctx context.Context, limit int) ([]Event, error)
}
