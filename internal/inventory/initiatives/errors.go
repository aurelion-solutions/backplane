// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package initiatives

import "errors"

// ErrNotFound is returned when a lookup or update targets an id that
// does not exist.
var ErrNotFound = errors.New("initiatives: not found")
