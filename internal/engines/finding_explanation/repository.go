// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package finding_explanation

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Repository is the persistence boundary for Explanation artifacts.
type Repository interface {
	Insert(ctx context.Context, row *Explanation) error
	Update(ctx context.Context, row *Explanation) error
	GetByID(ctx context.Context, id uuid.UUID) (*Explanation, error)
	// GetByFindingAndHash backs the cache: a fresh explanation for the
	// same (finding, input_hash) is reused instead of regenerated.
	GetByFindingAndHash(ctx context.Context, findingID uuid.UUID, inputHash string) (*Explanation, error)
	// GetLatestByFinding returns the most recently created explanation
	// for a finding.
	GetLatestByFinding(ctx context.Context, findingID uuid.UUID) (*Explanation, error)
}

// BunRepository is the production Postgres-backed Repository.
type BunRepository struct {
	db *bun.DB
}

// NewBunRepository constructs a BunRepository.
func NewBunRepository(db *bun.DB) *BunRepository {
	return &BunRepository{db: db}
}

// Insert writes a new explanation row.
func (r *BunRepository) Insert(ctx context.Context, row *Explanation) error {
	_, err := r.db.NewInsert().Model(row).Exec(ctx)
	return err
}

// Update rewrites a mutable explanation row by primary key (status →
// narrative / citations / error / completed_at as generation finishes).
func (r *BunRepository) Update(ctx context.Context, row *Explanation) error {
	_, err := r.db.NewUpdate().
		Model(row).
		WherePK().
		Column("status", "narrative", "citations", "model_ref", "error", "completed_at").
		Exec(ctx)
	return err
}

// GetByID returns one row by primary key.
func (r *BunRepository) GetByID(ctx context.Context, id uuid.UUID) (*Explanation, error) {
	row := new(Explanation)
	err := r.db.NewSelect().Model(row).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrExplanationNotFound
		}
		return nil, err
	}
	return row, nil
}

// GetByFindingAndHash returns the cached explanation for one finding +
// input hash, or ErrExplanationNotFound.
func (r *BunRepository) GetByFindingAndHash(ctx context.Context, findingID uuid.UUID, inputHash string) (*Explanation, error) {
	row := new(Explanation)
	err := r.db.NewSelect().
		Model(row).
		Where("finding_id = ?", findingID).
		Where("input_hash = ?", inputHash).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrExplanationNotFound
		}
		return nil, err
	}
	return row, nil
}

// GetLatestByFinding returns the newest explanation for a finding.
func (r *BunRepository) GetLatestByFinding(ctx context.Context, findingID uuid.UUID) (*Explanation, error) {
	row := new(Explanation)
	err := r.db.NewSelect().
		Model(row).
		Where("finding_id = ?", findingID).
		Order("created_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrExplanationNotFound
		}
		return nil, err
	}
	return row, nil
}
