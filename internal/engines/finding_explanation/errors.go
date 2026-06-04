// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package finding_explanation

import "errors"

var (
	// ErrFindingNotFound is returned when the target finding does not
	// exist.
	ErrFindingNotFound = errors.New("finding_explanation: finding not found")

	// ErrExplanationNotFound is returned when no explanation artifact
	// exists for the requested finding.
	ErrExplanationNotFound = errors.New("finding_explanation: explanation not found")

	// ErrGenerationFailed wraps a model-side failure. It is a generation
	// failure, never a finding failure — the finding stays deterministic.
	ErrGenerationFailed = errors.New("finding_explanation: generation failed")
)
