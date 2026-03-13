// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package llm

import "context"

// Stub is a no-op Provider whose Stream returns ErrNotImplemented.
// Embed in a provider whose real backend has not been wired.
type Stub struct{}

// Stream implements Provider.
func (Stub) Stream(_ context.Context, _ []Message, _ map[string]any) (<-chan Chunk, error) {
	return nil, ErrNotImplemented
}
