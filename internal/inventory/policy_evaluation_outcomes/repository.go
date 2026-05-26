// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policy_evaluation_outcomes

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// ListFilter narrows what List returns. Zero value lists every row.
type ListFilter struct {
	AssessmentRunID *uuid.UUID
	Outcome         string
	CartridgeID     string
	TargetType      string
	Limit           int
	Offset          int
}

// Repository is the persistence boundary for PolicyEvaluationOutcome
// rows. Upsert is idempotent on the identity tuple so a re-emission in
// the same run overwrites rather than duplicating.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*PolicyEvaluationOutcome, error)
	List(ctx context.Context, f ListFilter) ([]*PolicyEvaluationOutcome, int, error)

	Upsert(ctx context.Context, o *PolicyEvaluationOutcome) error
}

// BunRepository is the production Postgres-backed Repository.
type BunRepository struct {
	db *bun.DB
}

// NewBunRepository constructs a BunRepository.
func NewBunRepository(db *bun.DB) *BunRepository {
	return &BunRepository{db: db}
}

// GetByID returns one row by primary key.
func (r *BunRepository) GetByID(ctx context.Context, id uuid.UUID) (*PolicyEvaluationOutcome, error) {
	row := new(PolicyEvaluationOutcome)
	err := r.db.NewSelect().Model(row).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return row, nil
}

// List returns a paginated slice + total honouring the filter. Default
// ordering is evaluated_at DESC.
func (r *BunRepository) List(ctx context.Context, f ListFilter) ([]*PolicyEvaluationOutcome, int, error) {
	out := []*PolicyEvaluationOutcome{}
	q := r.db.NewSelect().Model(&out)
	if f.AssessmentRunID != nil {
		q = q.Where("assessment_run_id = ?", *f.AssessmentRunID)
	}
	if f.Outcome != "" {
		q = q.Where("outcome = ?", f.Outcome)
	}
	if f.CartridgeID != "" {
		q = q.Where("cartridge_id = ?", f.CartridgeID)
	}
	if f.TargetType != "" {
		q = q.Where("target_type = ?", f.TargetType)
	}
	q = q.Order("evaluated_at DESC")
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	if f.Offset > 0 {
		q = q.Offset(f.Offset)
	}
	total, err := q.ScanAndCount(ctx)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// Upsert inserts the outcome or, on identity-tuple conflict, overwrites
// the mutable columns (outcome, missing_evidence, source_id,
// evaluated_at) of the existing row. Idempotent within a run.
func (r *BunRepository) Upsert(ctx context.Context, row *PolicyEvaluationOutcome) error {
	_, err := r.db.NewInsert().
		Model(row).
		On("CONFLICT (assessment_run_id, cartridge_id, rule_id, target_type, target_key) DO UPDATE").
		Set("outcome = EXCLUDED.outcome").
		Set("missing_evidence = EXCLUDED.missing_evidence").
		Set("source_id = EXCLUDED.source_id").
		Set("evaluated_at = EXCLUDED.evaluated_at").
		Exec(ctx)
	return err
}
