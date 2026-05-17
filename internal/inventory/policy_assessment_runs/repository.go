// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policy_assessment_runs

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// ListFilter narrows what List returns. Zero value lists every row.
type ListFilter struct {
	Status             string
	TriggeredBy        string
	ScopePrincipalID   *uuid.UUID
	ScopeApplicationID *uuid.UUID
	Limit              int
	Offset             int
}

// Repository is the persistence boundary for AssessmentRun rows.
//
// Read paths (GetByID, List) are what the REST handlers and result
// readers call. The write path (Insert, Update) is what the worker
// policy-assessment action calls when it starts and finishes a pass.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*AssessmentRun, error)
	List(ctx context.Context, f ListFilter) ([]*AssessmentRun, int, error)

	Insert(ctx context.Context, r *AssessmentRun) error
	Update(ctx context.Context, r *AssessmentRun) error
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
func (r *BunRepository) GetByID(ctx context.Context, id uuid.UUID) (*AssessmentRun, error) {
	row := new(AssessmentRun)
	err := r.db.NewSelect().Model(row).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return row, nil
}

// List returns a paginated slice + total row count honouring the filter.
// Default ordering is created_at DESC — operators see most recent runs first.
func (r *BunRepository) List(ctx context.Context, f ListFilter) ([]*AssessmentRun, int, error) {
	out := []*AssessmentRun{}
	q := r.db.NewSelect().Model(&out)
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.TriggeredBy != "" {
		q = q.Where("triggered_by = ?", f.TriggeredBy)
	}
	if f.ScopePrincipalID != nil {
		q = q.Where("scope_principal_id = ?", *f.ScopePrincipalID)
	}
	if f.ScopeApplicationID != nil {
		q = q.Where("scope_application_id = ?", *f.ScopeApplicationID)
	}
	q = q.Order("created_at DESC")
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

// Insert writes a fresh assessment run row. Callers must populate ID,
// Status, TriggeredBy, FindingsBySeverity (at minimum empty map),
// CreatedAt.
func (r *BunRepository) Insert(ctx context.Context, row *AssessmentRun) error {
	_, err := r.db.NewInsert().Model(row).Exec(ctx)
	return err
}

// Update writes a full row update keyed by ID. Used by the worker
// policy-assessment action to transition pending → running →
// completed/failed and to stamp the final counters.
func (r *BunRepository) Update(ctx context.Context, row *AssessmentRun) error {
	res, err := r.db.NewUpdate().Model(row).WherePK().Exec(ctx)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
