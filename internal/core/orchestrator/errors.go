// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package orchestrator

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// ErrNotFound is returned when the requested row id does not exist.
var ErrNotFound = errors.New("orchestrator: not found")

// ErrAlreadyCancelling is returned by RequestCancel when the run is
// already in 'cancelling'.
var ErrAlreadyCancelling = errors.New("orchestrator: already cancelling")

// ErrTerminal is returned by RequestCancel / CreateRetry when the
// target run is already terminal.
var ErrTerminal = errors.New("orchestrator: run already terminal")

// ErrNotRetryable is returned by CreateRetry when the source run is
// in 'cancelling' or a non-terminal status. The Reason field gives a
// concise discriminator for routes layer.
type ErrNotRetryable struct {
	RunID  uuid.UUID
	Status RunStatus
	Reason string
}

func (e *ErrNotRetryable) Error() string {
	return fmt.Sprintf("orchestrator: run %s not retryable (%s, %s)", e.RunID, e.Status, e.Reason)
}

// ErrStateConflict is returned when a status-guarded UPDATE finds 0
// rows. Carries the expected source statuses and the actual status the
// row had at re-read time (nil when the row is gone entirely).
type ErrStateConflict struct {
	RunID    uuid.UUID
	Expected []RunStatus
	Actual   *RunStatus
}

func (e *ErrStateConflict) Error() string {
	actual := "missing"
	if e.Actual != nil {
		actual = string(*e.Actual)
	}
	return fmt.Sprintf("orchestrator: state conflict on run %s (expected %v, actual %s)",
		e.RunID, e.Expected, actual)
}
