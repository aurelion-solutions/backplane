// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package compliance_projection

import "errors"

// ErrProjectionNotFound is returned when no projection cartridge declares
// the requested projection id.
var ErrProjectionNotFound = errors.New("compliance_projection: projection not found")

// ErrControlNotFound is returned when a projection has no control with
// the requested control id.
var ErrControlNotFound = errors.New("compliance_projection: control not found")

// ErrRunNotFound is returned when the assessment run id is unknown.
var ErrRunNotFound = errors.New("compliance_projection: assessment run not found")

// ErrInvalidDefinition is returned when a projection.json fails to parse
// or omits a required field.
var ErrInvalidDefinition = errors.New("compliance_projection: invalid projection definition")
