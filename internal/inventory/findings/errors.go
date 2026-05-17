// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package findings

import "errors"

// ErrNotFound is returned when a finding row cannot be located.
var ErrNotFound = errors.New("findings: not found")
