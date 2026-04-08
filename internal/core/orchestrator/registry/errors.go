// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package registry

import "errors"

var (
	// ErrDuplicate is returned when the same (engine, action) pair is
	// registered twice.
	ErrDuplicate = errors.New("registry: duplicate (engine, action)")

	// ErrNotFound is returned when the runner dispatches a pair that
	// the registry does not know about.
	ErrNotFound = errors.New("registry: action not found")

	// ErrArgsValidation is returned when raw args fail the action's
	// JSON Schema.
	ErrArgsValidation = errors.New("registry: args validation failed")

	// ErrResultValidation is returned when the handler's return value
	// fails the action's result JSON Schema.
	ErrResultValidation = errors.New("registry: result validation failed")

	// ErrSchemaGeneration is returned when invopop fails to produce a
	// JSON Schema for the handler's input or output type. Should only
	// surface at composition time.
	ErrSchemaGeneration = errors.New("registry: schema generation failed")
)
