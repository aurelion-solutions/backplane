// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package org_units

import "errors"

// ErrNotFound is returned when a lookup misses.
var ErrNotFound = errors.New("org_units: not found")

// ErrExternalIDAlreadyExists is returned when a Create or BulkUpsert
// violates the unique (external_id) constraint.
var ErrExternalIDAlreadyExists = errors.New("org_units: external_id already exists")

// ErrParentNotFound is returned when a referenced parent_id / parent
// external_id does not resolve to an existing row.
var ErrParentNotFound = errors.New("org_units: parent not found")

// ErrParentInternal is returned when an attempt is made to attach an
// external org unit under an internal parent (or vice versa). Trees
// must not cross the is_internal boundary.
var ErrParentInternal = errors.New("org_units: parent must share the is_internal flag")

// ErrNoFields is returned when a patch payload carries no fields.
var ErrNoFields = errors.New("org_units: at least one field must be provided for update")

// ErrCannotDeleteInternal is returned when the HTTP layer tries to
// remove a row marked is_internal — only external nodes are writable
// via the API.
var ErrCannotDeleteInternal = errors.New("org_units: internal nodes are read-only via the API")

// ErrBulkEmpty is returned when bulk receives no items.
var ErrBulkEmpty = errors.New("org_units: bulk items must not be empty")

// ErrBulkTooLarge is returned when bulk size exceeds the per-request cap.
var ErrBulkTooLarge = errors.New("org_units: bulk size exceeds limit")

// ErrSelfReference is returned when a bulk item names itself as parent.
var ErrSelfReference = errors.New("org_units: parent_external_id cannot equal external_id")
