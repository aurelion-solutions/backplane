// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policies

import "errors"

// ErrNotFound is returned when the requested policy row does not exist.
var ErrNotFound = errors.New("policies: not found")
