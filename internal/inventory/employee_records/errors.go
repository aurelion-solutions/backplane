// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package employee_records

import "errors"

// ErrNotFound is returned when an employee_record lookup misses.
var ErrNotFound = errors.New("employee_records: not found")

// ErrApplicationNotFound is returned when the referenced application_id
// does not resolve.
var ErrApplicationNotFound = errors.New("employee_records: application not found")

// ErrPersonNotFound is returned when the referenced person_id (for a
// manual match) does not resolve.
var ErrPersonNotFound = errors.New("employee_records: person not found")

// ErrEmploymentNotFound is returned when the referenced employment_id
// (for a manual match) does not resolve.
var ErrEmploymentNotFound = errors.New("employee_records: employment not found")

// ErrDuplicate is returned when (application_id, external_id) collides.
var ErrDuplicate = errors.New("employee_records: (application_id, external_id) already exists")

// ErrAttributeNotFound is returned when an attribute key is unknown.
var ErrAttributeNotFound = errors.New("employee_records: attribute not found")

// ErrMappingNotFound is returned when a provider-attribute-mapping row
// does not exist.
var ErrMappingNotFound = errors.New("employee_records: provider attribute mapping not found")

// ErrMappingDuplicate is returned on a (application_id,
// employee_record_key) unique-violation.
var ErrMappingDuplicate = errors.New("employee_records: provider attribute mapping already exists")

// ErrBulkEmpty is returned when bulk receives no items.
var ErrBulkEmpty = errors.New("employee_records: bulk items must not be empty")

// ErrBulkTooLarge is returned when bulk size exceeds the per-request cap.
var ErrBulkTooLarge = errors.New("employee_records: bulk size exceeds limit")
