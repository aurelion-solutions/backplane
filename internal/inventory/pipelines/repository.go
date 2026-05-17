// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package pipelines

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// ListFilter narrows what List returns.
type ListFilter struct {
	CartridgeRef    string
	IncludeInactive bool
	Limit           int
	Offset          int
}

// Repository is the persistence boundary for Pipeline rows.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Pipeline, error)
	GetByNaturalKey(ctx context.Context, cartridgeRef, name string) (*Pipeline, error)
	List(ctx context.Context, f ListFilter) ([]*Pipeline, int, error)
	ListActiveByCartridge(ctx context.Context, cartridgeRef string) ([]*Pipeline, error)

	Upsert(ctx context.Context, p *Pipeline) error
	MarkRemoved(ctx context.Context, id uuid.UUID, removedAt time.Time) error
	Resurrect(ctx context.Context, id uuid.UUID, now time.Time) error
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
func (r *BunRepository) GetByID(ctx context.Context, id uuid.UUID) (*Pipeline, error) {
	p := new(Pipeline)
	err := r.db.NewSelect().Model(p).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return p, nil
}

// GetByNaturalKey returns one row by (cartridge_ref, name).
func (r *BunRepository) GetByNaturalKey(ctx context.Context, cartridgeRef, name string) (*Pipeline, error) {
	p := new(Pipeline)
	err := r.db.NewSelect().
		Model(p).
		Where("cartridge_ref = ?", cartridgeRef).
		Where("name = ?", name).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return p, nil
}

// List returns a paginated slice + total row count, honoring the filter.
func (r *BunRepository) List(ctx context.Context, f ListFilter) ([]*Pipeline, int, error) {
	out := []*Pipeline{}
	q := r.db.NewSelect().Model(&out)
	if f.CartridgeRef != "" {
		q = q.Where("cartridge_ref = ?", f.CartridgeRef)
	}
	if !f.IncludeInactive {
		q = q.Where("is_active = TRUE")
	}
	q = q.Order("cartridge_ref ASC", "name ASC")
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

// ListActiveByCartridge returns every active pipeline belonging to one
// cartridge — the sync manager uses this to diff against the
// cartridge's current definition set.
func (r *BunRepository) ListActiveByCartridge(ctx context.Context, cartridgeRef string) ([]*Pipeline, error) {
	out := []*Pipeline{}
	err := r.db.NewSelect().
		Model(&out).
		Where("cartridge_ref = ?", cartridgeRef).
		Where("is_active = TRUE").
		Order("name ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Upsert inserts a new row or updates the existing one keyed by
// (cartridge_ref, name). Always sets is_active=TRUE and clears
// removed_at — used by the sync loop both for fresh inserts and for
// resurrecting previously soft-deleted rows.
func (r *BunRepository) Upsert(ctx context.Context, p *Pipeline) error {
	_, err := r.db.NewInsert().
		Model(p).
		On("CONFLICT (cartridge_ref, name) DO UPDATE").
		Set("version      = EXCLUDED.version").
		Set("content_hash = EXCLUDED.content_hash").
		Set("source_path  = EXCLUDED.source_path").
		Set("is_active    = TRUE").
		Set("removed_at   = NULL").
		Set("meta         = EXCLUDED.meta").
		Set("updated_at   = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

// MarkRemoved flips an existing row to is_active=false and stamps
// removed_at.
func (r *BunRepository) MarkRemoved(ctx context.Context, id uuid.UUID, removedAt time.Time) error {
	res, err := r.db.NewUpdate().
		Model((*Pipeline)(nil)).
		Set("is_active = FALSE").
		Set("removed_at = ?", removedAt).
		Set("updated_at = ?", removedAt).
		Where("id = ?", id).
		Where("is_active = TRUE").
		Exec(ctx)
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

// Resurrect flips a soft-deleted row back to active.
func (r *BunRepository) Resurrect(ctx context.Context, id uuid.UUID, now time.Time) error {
	res, err := r.db.NewUpdate().
		Model((*Pipeline)(nil)).
		Set("is_active = TRUE").
		Set("removed_at = NULL").
		Set("updated_at = ?", now).
		Where("id = ?", id).
		Exec(ctx)
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
