// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package employments

import "errors"

// ErrNotFound is returned when an employment lookup misses.
var ErrNotFound = errors.New("employments: not found")

// ErrPersonNotFound is returned when the referenced person_id does
// not resolve.
var ErrPersonNotFound = errors.New("employments: person not found")

// ErrOrgUnitNotFound is returned when the referenced org_unit_id is
// missing.
var ErrOrgUnitNotFound = errors.New("employments: org_unit not found")

// ErrAttributeNotFound is returned when an attribute key is unknown.
var ErrAttributeNotFound = errors.New("employments: attribute not found")

// ErrNoFields is returned when a patch payload carries no fields.
var ErrNoFields = errors.New("employments: at least one field must be provided for update")

// ErrInvalidDates is returned when end_date < start_date.
var ErrInvalidDates = errors.New("employments: end_date must be on or after start_date")

// ErrCodeRequired is returned when the code field is empty or too long.
var ErrCodeRequired = errors.New("employments: code must be 1..64 characters")

// ErrAlreadyEnded is returned by End on a row whose end_date is already set.
var ErrAlreadyEnded = errors.New("employments: already ended")

// ErrBulkEmpty is returned when bulk receives no items.
var ErrBulkEmpty = errors.New("employments: bulk items must not be empty")

// ErrBulkTooLarge is returned when bulk size exceeds the per-request cap.
var ErrBulkTooLarge = errors.New("employments: bulk size exceeds limit")
