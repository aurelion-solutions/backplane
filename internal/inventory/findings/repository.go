// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package findings

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// ListFilter narrows what List returns. Zero value lists every row.
//
// PrincipalID / TargetType+TargetID / PolicyID / AssessmentRunID
// restrict to one anchor. Kind / Status / Severity restrict to the
// named value. Limit and Offset paginate; default ordering is
// detected_at DESC so the freshest findings surface first.
type ListFilter struct {
	PrincipalID     *uuid.UUID
	TargetType      string
	TargetID        *uuid.UUID
	ApplicationID   *uuid.UUID
	PolicyID        *uuid.UUID
	AssessmentRunID *uuid.UUID
	LastSeenRunID   *uuid.UUID
	Kind            string
	ExcludeKind     string
	Status          string
	Severity        string
	Source          string
	CartridgeRef    string
	Owner           string
	Limit           int
	Offset          int
}

// Repository is the persistence boundary for Finding rows.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Finding, error)
	List(ctx context.Context, f ListFilter) ([]*Finding, int, error)

	Insert(ctx context.Context, f *Finding) error
	// TouchLastSeen advances last_seen_run_id and evaluated_at on the
	// existing finding identified by evidenceHash. It is how a re-run
	// re-confirms a finding the idempotent Insert skipped as a duplicate.
	// Returns the number of rows updated (0 if no such finding).
	TouchLastSeen(ctx context.Context, evidenceHash string, runID uuid.UUID, evaluatedAt time.Time) (int, error)
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
func (r *BunRepository) GetByID(ctx context.Context, id uuid.UUID) (*Finding, error) {
	row := new(Finding)
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
func (r *BunRepository) List(ctx context.Context, f ListFilter) ([]*Finding, int, error) {
	out := []*Finding{}
	q := r.db.NewSelect().Model(&out)
	if f.PrincipalID != nil {
		q = q.Where("principal_id = ?", *f.PrincipalID)
	}
	if f.TargetType != "" {
		q = q.Where("target_type = ?", f.TargetType)
	}
	if f.TargetID != nil {
		q = q.Where("target_id = ?", *f.TargetID)
	}
	if f.ApplicationID != nil {
		q = q.Where("application_id = ?", *f.ApplicationID)
	}
	if f.PolicyID != nil {
		q = q.Where("policy_id = ?", *f.PolicyID)
	}
	if f.AssessmentRunID != nil {
		q = q.Where("assessment_run_id = ?", *f.AssessmentRunID)
	}
	if f.LastSeenRunID != nil {
		q = q.Where("last_seen_run_id = ?", *f.LastSeenRunID)
	}
	if f.Kind != "" {
		q = q.Where("kind = ?", f.Kind)
	}
	if f.ExcludeKind != "" {
		q = q.Where("kind != ?", f.ExcludeKind)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.Severity != "" {
		q = q.Where("severity = ?", f.Severity)
	}
	if f.Source != "" {
		q = q.Where("source = ?", f.Source)
	}
	if f.CartridgeRef != "" {
		q = q.Where("cartridge_ref = ?", f.CartridgeRef)
	}
	if f.Owner != "" {
		q = q.Where("owner_ref = ?", f.Owner)
	}
	// Highest-priority findings first, then freshest.
	q = q.Order("priority_score DESC").Order("detected_at DESC")
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

// Insert writes a fresh finding row. Callers must populate every
// non-nullable field; the unique constraint on the evidence tuple
// raises a duplicate-key error on a re-emission, which the caller can
// trap and treat as a reuse signal.
func (r *BunRepository) Insert(ctx context.Context, row *Finding) error {
	_, err := r.db.NewInsert().Model(row).Exec(ctx)
	return err
}

// TouchLastSeen advances last_seen_run_id and evaluated_at on the finding
// whose evidence_hash matches, re-confirming a finding that the idempotent
// Insert reported as a duplicate. evidence_hash is unique, so at most one
// row updates.
func (r *BunRepository) TouchLastSeen(ctx context.Context, evidenceHash string, runID uuid.UUID, evaluatedAt time.Time) (int, error) {
	res, err := r.db.NewUpdate().
		Model((*Finding)(nil)).
		Set("last_seen_run_id = ?", runID).
		Set("evaluated_at = ?", evaluatedAt).
		Where("evidence_hash = ?", evidenceHash).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}
