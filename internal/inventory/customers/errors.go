// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package customers

import "errors"

// ErrNotFound is returned when a lookup misses.
var ErrNotFound = errors.New("customers: not found")

// ErrExternalIDAlreadyExists is returned on a unique violation.
var ErrExternalIDAlreadyExists = errors.New("customers: external_id already exists")

// ErrInvalidEnum is returned when a tenant_role or plan_tier value is
// outside its allowed vocabulary.
var ErrInvalidEnum = errors.New("customers: enum value not allowed")

// ErrAttributeNotFound is returned when an attribute key is unknown.
var ErrAttributeNotFound = errors.New("customers: attribute not found")

// ErrNoFields is returned when a patch payload carries no fields.
var ErrNoFields = errors.New("customers: at least one field must be provided for update")

// ErrBulkEmpty is returned when bulk receives no items.
var ErrBulkEmpty = errors.New("customers: bulk items must not be empty")

// ErrBulkTooLarge is returned when bulk size exceeds the per-request cap.
var ErrBulkTooLarge = errors.New("customers: bulk size exceeds limit")
