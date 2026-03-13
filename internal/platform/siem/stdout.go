// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package siem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// Stdout is a Sink that writes one JSON-encoded Event per line to
// os.Stdout. Useful for local dev when running binaries in a terminal
// and watching events fly by.
//
// Writes are serialised by an internal mutex; safe for concurrent Emit.
type Stdout struct {
	mu sync.Mutex
}

// NewStdout returns a fresh Stdout sink.
func NewStdout() *Stdout { return &Stdout{} }

// Emit implements Sink.
func (s *Stdout) Emit(_ context.Context, event Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("siem/stdout: marshal: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := os.Stdout.Write(body); err != nil {
		return fmt.Errorf("siem/stdout: write: %w", err)
	}
	if _, err := os.Stdout.Write([]byte("\n")); err != nil {
		return fmt.Errorf("siem/stdout: write: %w", err)
	}
	return nil
}

// RegisterStdout wires the "stdout" provider into f.
func RegisterStdout(f *Factory) {
	f.Register("stdout", func() (Sink, error) { return NewStdout(), nil })
}
