// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package events

import "context"

// Tee delegates Emit to a primary Sink, then to each tap in order.
//
// The primary carries the load-bearing contract (publish to the
// events exchange). Errors from the primary propagate unchanged.
//
// Taps are observability, not load-bearing. Errors from taps are
// silently dropped so a failing tap never blocks the primary.
type Tee struct {
	primary Sink
	taps    []Sink
}

// NewTee composes a primary Sink with zero or more observability taps.
func NewTee(primary Sink, taps ...Sink) *Tee {
	return &Tee{primary: primary, taps: taps}
}

// Emit implements Sink.
func (t *Tee) Emit(ctx context.Context, e Envelope) error {
	if err := t.primary.Emit(ctx, e); err != nil {
		return err
	}
	for _, tap := range t.taps {
		_ = tap.Emit(ctx, e)
	}
	return nil
}
