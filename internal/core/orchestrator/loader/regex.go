// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package loader

import "regexp"

// argNameRe enforces the same identifier shape the grammar uses for
// arg names — used by mq trigger args_from_payload validation.
var argNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
