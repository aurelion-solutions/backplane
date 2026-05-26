// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package consent

import "errors"

// ErrInvalidPayload is returned when a lake record is missing the fields
// required to persist a consent entity (source, the verifiable anchor /
// external_id, and grant_type for a grant).
var ErrInvalidPayload = errors.New("consent: invalid payload")

// ErrNotFound is returned by the lookups when no matching row exists.
var ErrNotFound = errors.New("consent: not found")
