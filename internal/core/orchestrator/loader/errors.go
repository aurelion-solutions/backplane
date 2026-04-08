// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package loader

import "errors"

// ErrLoad is the base wrapper for any loader failure. Specific sentinels
// below are wrapped through it (errors.Is matches both).
var ErrLoad = errors.New("orchestrator/loader: load failed")

// ErrSchema is returned when a pipeline YAML violates the JSON Schema
// grammar in internal/core/orchestrator/grammar.
var ErrSchema = errors.New("orchestrator/loader: schema violation")

// ErrActionRef is returned when a step references an (engine, action)
// pair not present in the live registry. Only raised when the loader
// is configured with ValidateActionRefs = true and a registry is
// attached.
var ErrActionRef = errors.New("orchestrator/loader: unknown action ref")

// ErrRequiresOrder is returned when a step's requires[] entry is a
// forward / self / unknown reference.
var ErrRequiresOrder = errors.New("orchestrator/loader: bad requires order")

// ErrTemplating is returned when a ${...} expression names an
// undeclared pipeline arg or a step outside the transitive requires
// closure.
var ErrTemplating = errors.New("orchestrator/loader: invalid template ref")

// ErrTrigger is returned when a trigger declaration is semantically
// invalid (duplicate schedule, malformed args_from_payload, etc.).
var ErrTrigger = errors.New("orchestrator/loader: invalid trigger")

// ErrDuplicateName is returned when two pipeline YAML files declare
// the same pipeline.name across a directory or LoadMany set.
var ErrDuplicateName = errors.New("orchestrator/loader: duplicate pipeline name")
