// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package workloads

import "errors"

// ErrNotFound is returned when a lookup misses.
var ErrNotFound = errors.New("workloads: not found")

// ErrExternalIDAlreadyExists is returned on unique violation.
var ErrExternalIDAlreadyExists = errors.New("workloads: external_id already exists")

// ErrOwnerNotFound is returned when the referenced owner_employment_id
// does not resolve.
var ErrOwnerNotFound = errors.New("workloads: owner employment not found")

// ErrApplicationNotFound is returned when the referenced application_id
// does not resolve.
var ErrApplicationNotFound = errors.New("workloads: application not found")

// ErrAttributeNotFound is returned when an attribute key is unknown.
var ErrAttributeNotFound = errors.New("workloads: attribute not found")

// ErrNoFields is returned when a patch payload carries no fields.
var ErrNoFields = errors.New("workloads: at least one field must be provided for update")

// ErrBulkEmpty is returned when bulk receives no items.
var ErrBulkEmpty = errors.New("workloads: bulk items must not be empty")

// ErrBulkTooLarge is returned when bulk size exceeds the per-request cap.
var ErrBulkTooLarge = errors.New("workloads: bulk size exceeds limit")
