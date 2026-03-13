// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package siem

import "context"

// MultiSink fan-outs every Emit to N child Sinks. Every child is
// called even if an earlier one errored; the first error is returned
// so the caller knows something went wrong without losing later
// deliveries.
//
// Order is preserved: children are called in registration order, on
// the same goroutine, sequentially.
type MultiSink struct {
	sinks []Sink
}

// NewMulti composes multiple Sinks into one. Calling NewMulti with no
// children yields a Sink whose Emit is a no-op.
func NewMulti(sinks ...Sink) *MultiSink {
	return &MultiSink{sinks: sinks}
}

// Emit implements Sink.
func (m *MultiSink) Emit(ctx context.Context, event Event) error {
	var firstErr error
	for _, s := range m.sinks {
		if err := s.Emit(ctx, event); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
