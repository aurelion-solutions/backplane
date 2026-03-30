// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package org_units

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Repository is the persistence boundary for OrgUnit aggregates.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*OrgUnit, error)
	GetByExternalID(ctx context.Context, externalID string) (*OrgUnit, error)
	List(ctx context.Context, limit, offset int) ([]*OrgUnit, int, error)
	Insert(ctx context.Context, u *OrgUnit) error
	Update(ctx context.Context, u *OrgUnit) error
	Delete(ctx context.Context, id uuid.UUID) error

	// BulkUpsert reconciles a batch of (external_id, parent_external_id)
	// items in one transaction. Two passes: pass-1 inserts every row
	// with parent_id=NULL so the unique (external_id) constraint
	// resolves ids; pass-2 walks the items again and patches parent_id
	// from the lookup table built in pass-1. Returns row count.
	BulkUpsert(ctx context.Context, items []BulkItem, idGen func() uuid.UUID) (int, error)
}

// BunRepository is the production Postgres-backed implementation.
type BunRepository struct {
	db  *bun.DB
	now func() time.Time
}

// NewBunRepository constructs a BunRepository.
func NewBunRepository(db *bun.DB, now func() time.Time) *BunRepository {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &BunRepository{db: db, now: now}
}

// GetByID returns one row by primary key, or ErrNotFound.
func (r *BunRepository) GetByID(ctx context.Context, id uuid.UUID) (*OrgUnit, error) {
	u := new(OrgUnit)
	err := r.db.NewSelect().Model(u).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return u, nil
}

// GetByExternalID returns one row by external_id, or ErrNotFound.
func (r *BunRepository) GetByExternalID(ctx context.Context, externalID string) (*OrgUnit, error) {
	u := new(OrgUnit)
	err := r.db.NewSelect().Model(u).Where("external_id = ?", externalID).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return u, nil
}

// List returns a paginated slice ordered by external_id ASC.
func (r *BunRepository) List(ctx context.Context, limit, offset int) ([]*OrgUnit, int, error) {
	out := []*OrgUnit{}
	q := r.db.NewSelect().Model(&out).Order("external_id ASC")
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

// Insert persists a new OrgUnit row.
func (r *BunRepository) Insert(ctx context.Context, u *OrgUnit) error {
	_, err := r.db.NewInsert().Model(u).Exec(ctx)
	return err
}

// Update writes back mutable columns.
func (r *BunRepository) Update(ctx context.Context, u *OrgUnit) error {
	_, err := r.db.NewUpdate().
		Model(u).
		Column("name", "description", "updated_at").
		Where("id = ?", u.ID).
		Exec(ctx)
	return err
}

// Delete removes a row by id.
func (r *BunRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.NewDelete().Model((*OrgUnit)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

// BulkUpsert reconciles a batch of external nodes (is_internal=false).
// All input items are external by definition — the REST API never
// writes internal nodes.
func (r *BunRepository) BulkUpsert(ctx context.Context, items []BulkItem, idGen func() uuid.UUID) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	now := r.now()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	// Pass 1: insert/update rows with parent_id = NULL. The unique
	// (external_id) constraint makes this idempotent on re-runs.
	for _, it := range items {
		u := &OrgUnit{
			ID:         idGen(),
			ExternalID: it.ExternalID,
			Name:       it.Name,
			IsInternal: false,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		u.Description = it.Description
		_, err := tx.NewInsert().Model(u).
			On("CONFLICT (external_id) DO UPDATE").
			Set("name = EXCLUDED.name").
			Set("description = EXCLUDED.description").
			Set("updated_at = EXCLUDED.updated_at").
			Exec(ctx)
		if err != nil {
			return 0, err
		}
	}

	// Build external_id → id lookup from the freshly committed rows.
	ids := map[string]uuid.UUID{}
	rows := []*OrgUnit{}
	if err := tx.NewSelect().Model(&rows).Column("id", "external_id").
		Where("external_id IN (?)", bun.In(extractExternalIDs(items))).
		Scan(ctx); err != nil {
		return 0, err
	}
	for _, row := range rows {
		ids[row.ExternalID] = row.ID
	}

	// Pass 2: stamp parent_id where parent_external_id is given.
	for _, it := range items {
		if it.ParentExternalID == nil {
			// Clear parent if absent — bulk wipes the field for the
			// node so re-rooting works without an explicit null hint.
			_, err := tx.NewUpdate().
				Model((*OrgUnit)(nil)).
				Set("parent_id = NULL").
				Set("updated_at = ?", now).
				Where("external_id = ?", it.ExternalID).
				Exec(ctx)
			if err != nil {
				return 0, err
			}
			continue
		}
		parentID, ok := ids[*it.ParentExternalID]
		if !ok {
			return 0, ErrParentNotFound
		}
		_, err := tx.NewUpdate().
			Model((*OrgUnit)(nil)).
			Set("parent_id = ?", parentID).
			Set("updated_at = ?", now).
			Where("external_id = ?", it.ExternalID).
			Exec(ctx)
		if err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(items), nil
}

func extractExternalIDs(items []BulkItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.ExternalID
	}
	return out
}
