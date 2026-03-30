// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package persons

import "errors"

// ErrNotFound is returned when a lookup by id (or external_id) misses.
var ErrNotFound = errors.New("persons: not found")

// ErrExternalIDAlreadyExists is returned when a Create violates the
// unique (external_id) constraint.
var ErrExternalIDAlreadyExists = errors.New("persons: external_id already exists")

// ErrAttributeNotFound is returned when an attribute key is unknown.
var ErrAttributeNotFound = errors.New("persons: attribute not found")

// ErrBulkTooLarge is returned when the bulk endpoint receives more
// items than the per-request cap allows.
var ErrBulkTooLarge = errors.New("persons: bulk size exceeds limit")

// ErrBulkEmpty is returned when the bulk endpoint receives an empty
// item list.
var ErrBulkEmpty = errors.New("persons: bulk items must not be empty")
