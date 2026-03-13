// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package siem

import "context"

// Stub is a no-op Sink whose Emit always returns ErrNotImplemented.
// Embed it (or alias to it) when sketching a provider whose real
// transport has not been wired yet.
type Stub struct{}

// Emit implements Sink.
func (Stub) Emit(_ context.Context, _ Event) error {
	return ErrNotImplemented
}
