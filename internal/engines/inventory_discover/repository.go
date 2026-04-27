// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package inventory_discover

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Repository is the persistence boundary for DiscoverRun.
type Repository interface {
	Insert(ctx context.Context, r *DiscoverRun) error
	Update(ctx context.Context, r *DiscoverRun) error
	GetByID(ctx context.Context, id uuid.UUID) (*DiscoverRun, error)
	GetByCorrelationID(ctx context.Context, correlationID string) (*DiscoverRun, error)
	List(ctx context.Context, limit, offset int) ([]*DiscoverRun, int, error)
}

// BunRepository is the Postgres-backed implementation.
type BunRepository struct{ db *bun.DB }

// NewBunRepository constructs a BunRepository.
func NewBunRepository(db *bun.DB) *BunRepository {
	return &BunRepository{db: db}
}

// Insert persists a freshly dispatched run.
func (r *BunRepository) Insert(ctx context.Context, run *DiscoverRun) error {
	_, err := r.db.NewInsert().Model(run).Exec(ctx)
	return err
}

// Update writes mutable columns on an existing run.
func (r *BunRepository) Update(ctx context.Context, run *DiscoverRun) error {
	_, err := r.db.NewUpdate().
		Model(run).
		Column("status", "error", "received_count", "written_count", "completed_at").
		Where("id = ?", run.ID).
		Exec(ctx)
	return err
}

// GetByID returns one run by primary key.
func (r *BunRepository) GetByID(ctx context.Context, id uuid.UUID) (*DiscoverRun, error) {
	run := new(DiscoverRun)
	err := r.db.NewSelect().Model(run).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return run, nil
}

// GetByCorrelationID returns the run dispatched with the given
// correlation_id. Used by the subscriber to route connector lifecycle
// events back to the originating run row.
func (r *BunRepository) GetByCorrelationID(ctx context.Context, correlationID string) (*DiscoverRun, error) {
	run := new(DiscoverRun)
	err := r.db.NewSelect().Model(run).Where("correlation_id = ?", correlationID).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return run, nil
}

// List returns paginated runs ordered by started_at DESC + total
// count.
func (r *BunRepository) List(ctx context.Context, limit, offset int) ([]*DiscoverRun, int, error) {
	out := []*DiscoverRun{}
	q := r.db.NewSelect().Model(&out).Order("started_at DESC", "id ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	total, err := q.ScanAndCount(ctx)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}
