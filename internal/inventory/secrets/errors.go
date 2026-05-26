// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package secrets

import "errors"

// ErrInvalidPayload is returned when a lake record is missing the
// fields required to persist a secret (source, external_id, type/format,
// and at least one locus).
var ErrInvalidPayload = errors.New("secrets: invalid payload")

// ErrNotFound is returned by the lookups when no matching secret exists.
var ErrNotFound = errors.New("secrets: not found")
