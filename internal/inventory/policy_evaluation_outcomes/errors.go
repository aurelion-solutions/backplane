// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policy_evaluation_outcomes

import "errors"

// ErrNotFound is returned when an outcome row cannot be located.
var ErrNotFound = errors.New("policy_evaluation_outcomes: not found")

// ErrInvalidOutcome is returned when a RecordOutcome call violates the
// biconditional (outcome = not_evaluable ⇔ missing_evidence non-empty)
// or carries an unknown outcome / target_type.
var ErrInvalidOutcome = errors.New("policy_evaluation_outcomes: invalid outcome")
