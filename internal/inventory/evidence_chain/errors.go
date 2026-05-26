// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package evidence_chain

import "errors"

// ErrNotFound is returned when an evidence-chain row cannot be located.
var ErrNotFound = errors.New("evidence_chain: not found")
