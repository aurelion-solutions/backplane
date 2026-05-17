// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package pipelines

import "errors"

// ErrNotFound is returned when the requested pipeline row does not exist.
var ErrNotFound = errors.New("pipelines: not found")
