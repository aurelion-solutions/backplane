// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package storage

import "context"

// Stub is a no-op Storage that returns ErrNotImplemented from every
// method. Embed in stub providers whose real backend has not been wired.
type Stub struct{}

// WriteBatch implements Storage.
func (Stub) WriteBatch(_ context.Context, _ string, _ []map[string]any) (string, error) {
	return "", ErrNotImplemented
}

// ReadBatch implements Storage.
func (Stub) ReadBatch(_ context.Context, _ string) ([]map[string]any, error) {
	return nil, ErrNotImplemented
}

// DeleteBatch implements Storage.
func (Stub) DeleteBatch(_ context.Context, _ string) error {
	return ErrNotImplemented
}

// AntiJoin implements Storage.
func (Stub) AntiJoin(_ context.Context, _ string, _ []Candidate) (AntiJoinResult, error) {
	return AntiJoinResult{}, ErrNotImplemented
}
