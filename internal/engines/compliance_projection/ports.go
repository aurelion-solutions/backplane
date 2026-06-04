// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package compliance_projection

import (
	"context"

	"github.com/google/uuid"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
	"github.com/aurelion-solutions/backplane/internal/inventory/findings"
	"github.com/aurelion-solutions/backplane/internal/inventory/policy_assessment_runs"
	"github.com/aurelion-solutions/backplane/internal/inventory/policy_evaluation_outcomes"
)

// CartridgeReader is the narrow slice of the cartridges provider this
// engine consumes. The platform's FilesystemProvider satisfies it; the
// engine never reaches past these three methods and never decides how a
// cartridge is materialised.
type CartridgeReader interface {
	List() ([]cartridges.Ref, error)
	Materialize(cartridges.Ref) (string, error)
	Policies(cartridges.Ref) (map[string]cartridges.Manifest, error)
}

// FindingsReader reads the findings present in an assessment run's
// current-posture baseline.
type FindingsReader interface {
	List(ctx context.Context, f findings.ListFilter) ([]*findings.Finding, int, error)
}

// OutcomesReader reads the policy-evaluation outcomes a run produced —
// the signal that a population was actually evaluated (matched /
// not_matched) versus left a gap (not_evaluable).
type OutcomesReader interface {
	List(ctx context.Context, f policy_evaluation_outcomes.ListFilter) ([]*policy_evaluation_outcomes.PolicyEvaluationOutcome, int, error)
}

// RunReader reads assessment-run metadata (status, time window) so a
// projection can state the period it covers.
type RunReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*policy_assessment_runs.AssessmentRun, error)
}
