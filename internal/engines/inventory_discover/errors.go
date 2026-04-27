// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package inventory_discover

import "errors"

// ErrNotFound is returned when a run lookup misses.
var ErrNotFound = errors.New("inventory_discover: run not found")

// ErrInvalidEnvelope is returned when the request envelope fails
// shape validation.
var ErrInvalidEnvelope = errors.New("inventory_discover: invalid envelope")

// ErrDispatch wraps a failure to publish the command to the connector.
var ErrDispatch = errors.New("inventory_discover: failed to dispatch command")
