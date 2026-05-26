// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package access_profile

import "errors"

// ErrPersonNotFound is returned by the service when no person matches
// the requested id.
var ErrPersonNotFound = errors.New("access_profile: person not found")
