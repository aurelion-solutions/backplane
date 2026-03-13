// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package events

import "context"

// Sink delivers a domain Envelope to a backend transport.
// Implementations must be safe for concurrent use.
type Sink interface {
	Emit(ctx context.Context, e Envelope) error
}
