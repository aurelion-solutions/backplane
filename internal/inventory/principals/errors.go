// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package principals

import "errors"

// ErrNotFound is returned when a principal lookup misses.
var ErrNotFound = errors.New("principals: not found")

// ErrDuplicate is returned when (kind, external_id) collides.
var ErrDuplicate = errors.New("principals: (kind, external_id) already exists")

// ErrBodyAlreadyBound is returned when an attempt is made to create a
// principal for a body that already owns one. The partial unique
// indexes on principal_*_id enforce this at the DB level.
var ErrBodyAlreadyBound = errors.New("principals: body is already bound to a principal")

// ErrBodyNotFound is returned when the requested body id does not
// resolve in its owning slice (employments / workloads / customers).
var ErrBodyNotFound = errors.New("principals: body not found")

// ErrInvalidKind is returned when an unknown kind value is supplied.
var ErrInvalidKind = errors.New("principals: invalid kind")

// ErrPrincipalMissingForBody is returned by RecomputeForBody when no
// principal row exists for the given (kind, body_id).
var ErrPrincipalMissingForBody = errors.New("principals: no principal for body")
