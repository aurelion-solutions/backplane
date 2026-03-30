// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package workloads

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Repository is the persistence boundary for Workload aggregates.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Workload, error)
	GetByExternalID(ctx context.Context, externalID string) (*Workload, error)
	List(ctx context.Context, limit, offset int) ([]*Workload, int, error)
	Insert(ctx context.Context, w *Workload) error
	Update(ctx context.Context, w *Workload) error

	ListAttributes(ctx context.Context, workloadID uuid.UUID) ([]*WorkloadAttribute, error)
	GetAttribute(ctx context.Context, workloadID uuid.UUID, key string) (*WorkloadAttribute, error)
	UpsertAttribute(ctx context.Context, a *WorkloadAttribute) error
	DeleteAttribute(ctx context.Context, workloadID uuid.UUID, key string) error

	BulkUpsert(ctx context.Context, items []BulkItem, idGen func() uuid.UUID) (int, error)
}

// BunRepository is the production Postgres-backed implementation.
type BunRepository struct {
	db *bun.DB
}

// NewBunRepository constructs a BunRepository.
func NewBunRepository(db *bun.DB) *BunRepository {
	return &BunRepository{db: db}
}

// GetByID returns one row by primary key.
func (r *BunRepository) GetByID(ctx context.Context, id uuid.UUID) (*Workload, error) {
	w := new(Workload)
	err := r.db.NewSelect().Model(w).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return w, nil
}

// GetByExternalID returns one row by external_id.
func (r *BunRepository) GetByExternalID(ctx context.Context, externalID string) (*Workload, error) {
	w := new(Workload)
	err := r.db.NewSelect().Model(w).Where("external_id = ?", externalID).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return w, nil
}

// List returns a paginated slice + total row count.
func (r *BunRepository) List(ctx context.Context, limit, offset int) ([]*Workload, int, error) {
	out := []*Workload{}
	q := r.db.NewSelect().Model(&out).Order("updated_at DESC", "id ASC")
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

// Insert persists a new Workload row.
func (r *BunRepository) Insert(ctx context.Context, w *Workload) error {
	_, err := r.db.NewInsert().Model(w).Exec(ctx)
	return err
}

// Update writes back mutable columns.
func (r *BunRepository) Update(ctx context.Context, w *Workload) error {
	_, err := r.db.NewUpdate().
		Model(w).
		Column("name", "description", "owner_employment_id", "application_id").
		Where("id = ?", w.ID).
		Exec(ctx)
	return err
}

// ListAttributes returns every attribute of a workload.
func (r *BunRepository) ListAttributes(ctx context.Context, workloadID uuid.UUID) ([]*WorkloadAttribute, error) {
	out := []*WorkloadAttribute{}
	err := r.db.NewSelect().Model(&out).Where("workload_id = ?", workloadID).Order("key ASC").Scan(ctx)
	return out, err
}

// GetAttribute returns one (workload, key) attribute.
func (r *BunRepository) GetAttribute(ctx context.Context, workloadID uuid.UUID, key string) (*WorkloadAttribute, error) {
	a := new(WorkloadAttribute)
	err := r.db.NewSelect().Model(a).
		Where("workload_id = ?", workloadID).
		Where("key = ?", key).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAttributeNotFound
		}
		return nil, err
	}
	return a, nil
}

// UpsertAttribute inserts or updates the (workload, key) row.
func (r *BunRepository) UpsertAttribute(ctx context.Context, a *WorkloadAttribute) error {
	_, err := r.db.NewInsert().
		Model(a).
		On("CONFLICT (workload_id, key) DO UPDATE").
		Set("value = EXCLUDED.value").
		Exec(ctx)
	return err
}

// DeleteAttribute removes one (workload, key) row.
func (r *BunRepository) DeleteAttribute(ctx context.Context, workloadID uuid.UUID, key string) error {
	res, err := r.db.NewDelete().
		Model((*WorkloadAttribute)(nil)).
		Where("workload_id = ?", workloadID).
		Where("key = ?", key).
		Exec(ctx)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrAttributeNotFound
	}
	return nil
}

// BulkUpsert reconciles a batch of workloads in one transaction.
func (r *BunRepository) BulkUpsert(ctx context.Context, items []BulkItem, idGen func() uuid.UUID) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	for _, it := range items {
		w := &Workload{
			ID:                idGen(),
			ExternalID:        it.ExternalID,
			Name:              it.Name,
			Description:       it.Description,
			OwnerEmploymentID: it.OwnerEmploymentID,
			ApplicationID:     it.ApplicationID,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		_, err := tx.NewInsert().Model(w).
			On("CONFLICT (external_id) DO UPDATE").
			Set("name                = EXCLUDED.name").
			Set("description         = EXCLUDED.description").
			Set("owner_employment_id = EXCLUDED.owner_employment_id").
			Set("application_id      = EXCLUDED.application_id").
			Set("updated_at          = EXCLUDED.updated_at").
			Returning("id").
			Exec(ctx)
		if err != nil {
			return 0, err
		}
		for k, v := range it.Attributes {
			a := &WorkloadAttribute{
				ID:         idGen(),
				WorkloadID: w.ID,
				Key:        k,
				Value:      v,
			}
			_, err := tx.NewInsert().Model(a).
				On("CONFLICT (workload_id, key) DO UPDATE").
				Set("value = EXCLUDED.value").
				Exec(ctx)
			if err != nil {
				return 0, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(items), nil
}
