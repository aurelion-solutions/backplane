// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package inventory_ingest

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Repository is the persistence boundary for IngestBatch audit rows.
type Repository interface {
	Insert(ctx context.Context, b *IngestBatch) error
	GetByID(ctx context.Context, id uuid.UUID) (*IngestBatch, error)
	List(ctx context.Context, limit, offset int) ([]*IngestBatch, int, error)
}

// BunRepository is the Postgres-backed implementation.
type BunRepository struct{ db *bun.DB }

// NewBunRepository constructs a BunRepository.
func NewBunRepository(db *bun.DB) *BunRepository {
	return &BunRepository{db: db}
}

// Insert persists a freshly completed audit row.
func (r *BunRepository) Insert(ctx context.Context, b *IngestBatch) error {
	_, err := r.db.NewInsert().Model(b).Exec(ctx)
	return err
}

// GetByID returns one row by primary key.
func (r *BunRepository) GetByID(ctx context.Context, id uuid.UUID) (*IngestBatch, error) {
	b := new(IngestBatch)
	err := r.db.NewSelect().Model(b).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return b, nil
}

// List returns a paginated slice ordered by completed_at DESC plus
// the total row count.
func (r *BunRepository) List(ctx context.Context, limit, offset int) ([]*IngestBatch, int, error) {
	out := []*IngestBatch{}
	q := r.db.NewSelect().Model(&out).Order("completed_at DESC", "id ASC")
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
