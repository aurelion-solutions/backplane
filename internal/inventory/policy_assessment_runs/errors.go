// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policy_assessment_runs

import "errors"

// ErrNotFound is returned when an assessment-run row cannot be located.
var ErrNotFound = errors.New("policy_assessment_runs: not found")
