// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policy_evaluation_outcomes

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Service is the business boundary for recording evaluation outcomes.
// It enforces the biconditional and the closed sets before persisting.
type Service struct {
	repo Repository
}

// NewService constructs a Service over the given repository.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// RecordParams is the input to RecordOutcome. ID and EvaluatedAt are
// filled in by the service when zero.
type RecordParams struct {
	AssessmentRunID uuid.UUID
	CartridgeID     string
	RuleID          string
	TargetType      string
	TargetRef       *uuid.UUID
	TargetKey       string
	Outcome         string
	MissingEvidence []string
	SourceID        *uuid.UUID
}

// RecordOutcome validates and idempotently persists one ternary
// outcome. It rejects unknown outcome / target_type values and any
// violation of the biconditional (outcome = not_evaluable ⇔
// missing_evidence non-empty), returning ErrInvalidOutcome.
func (s *Service) RecordOutcome(ctx context.Context, p RecordParams) (*PolicyEvaluationOutcome, error) {
	if err := validate(p); err != nil {
		return nil, err
	}
	row := &PolicyEvaluationOutcome{
		ID:              uuid.New(),
		AssessmentRunID: p.AssessmentRunID,
		CartridgeID:     p.CartridgeID,
		RuleID:          p.RuleID,
		TargetType:      p.TargetType,
		TargetRef:       p.TargetRef,
		TargetKey:       p.TargetKey,
		Outcome:         p.Outcome,
		MissingEvidence: p.MissingEvidence,
		SourceID:        p.SourceID,
		EvaluatedAt:     time.Now().UTC(),
	}
	if row.MissingEvidence == nil {
		row.MissingEvidence = []string{}
	}
	if err := s.repo.Upsert(ctx, row); err != nil {
		return nil, fmt.Errorf("record outcome: %w", err)
	}
	return row, nil
}

func validate(p RecordParams) error {
	switch p.Outcome {
	case OutcomeMatched, OutcomeNotMatched, OutcomeNotEvaluable:
	default:
		return fmt.Errorf("%w: outcome %q", ErrInvalidOutcome, p.Outcome)
	}
	switch p.TargetType {
	case TargetAccount, TargetSubject, TargetWorkload, TargetSource, TargetPipeline:
	default:
		return fmt.Errorf("%w: target_type %q", ErrInvalidOutcome, p.TargetType)
	}
	hasMissing := len(p.MissingEvidence) > 0
	notEval := p.Outcome == OutcomeNotEvaluable
	if notEval != hasMissing {
		return fmt.Errorf(
			"%w: not_evaluable ⇔ missing_evidence (outcome=%q, missing=%d)",
			ErrInvalidOutcome, p.Outcome, len(p.MissingEvidence),
		)
	}
	// Identity is the (run, cartridge, rule, target_type, target_key)
	// tuple — target_key is always required; target_ref is set only for
	// concrete entity targets (account/subject/workload).
	if p.AssessmentRunID == uuid.Nil || p.TargetKey == "" {
		return fmt.Errorf("%w: assessment_run_id and target_key are required", ErrInvalidOutcome)
	}
	return nil
}
