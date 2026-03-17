// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package applications

import "errors"

// ErrNotFound is returned when an Application with the requested
// identifier (id or code) does not exist.
var ErrNotFound = errors.New("applications: not found")

// ErrCodeAlreadyExists is returned when a Create or Update violates the
// unique (code) constraint. Service layer translates the Postgres
// 23505/uq_applications_code error into this typed value.
var ErrCodeAlreadyExists = errors.New("applications: code already exists")

// ErrNoFields is returned by Update when the patch carries no fields.
var ErrNoFields = errors.New("applications: at least one field must be provided for update")
