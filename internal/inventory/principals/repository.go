// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package principals

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/aurelion-solutions/backplane/internal/inventory/shared"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Repository is the persistence boundary for Principal aggregates.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Principal, error)
	GetByBody(ctx context.Context, kind shared.PrincipalKind, bodyID uuid.UUID) (*Principal, error)
	List(ctx context.Context, limit, offset int) ([]*Principal, int, error)
	Insert(ctx context.Context, p *Principal) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status string, updatedAt time.Time) error
	UpdateLock(ctx context.Context, id uuid.UUID, isLocked bool, updatedAt time.Time) error
	ListAttributes(ctx context.Context, principalID uuid.UUID) ([]*PrincipalAttribute, error)
}

// BunRepository is the production Postgres-backed implementation.
type BunRepository struct {
	db *bun.DB
}

// NewBunRepository constructs a BunRepository.
func NewBunRepository(db *bun.DB) *BunRepository {
	return &BunRepository{db: db}
}

// GetByID returns one Principal row.
func (r *BunRepository) GetByID(ctx context.Context, id uuid.UUID) (*Principal, error) {
	p := new(Principal)
	err := r.db.NewSelect().Model(p).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return p, nil
}

// GetByBody returns the unique Principal row tied to a body.
func (r *BunRepository) GetByBody(ctx context.Context, kind shared.PrincipalKind, bodyID uuid.UUID) (*Principal, error) {
	p := new(Principal)
	q := r.db.NewSelect().Model(p).Where("kind = ?", kind)
	switch kind {
	case shared.PrincipalKindEmployment:
		q = q.Where("principal_employment_id = ?", bodyID)
	case shared.PrincipalKindWorkload:
		q = q.Where("principal_workload_id = ?", bodyID)
	case shared.PrincipalKindCustomer:
		q = q.Where("principal_customer_id = ?", bodyID)
	default:
		return nil, ErrInvalidKind
	}
	err := q.Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return p, nil
}

// List returns a paginated slice + total row count.
func (r *BunRepository) List(ctx context.Context, limit, offset int) ([]*Principal, int, error) {
	out := []*Principal{}
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

// Insert persists a new Principal row.
func (r *BunRepository) Insert(ctx context.Context, p *Principal) error {
	_, err := r.db.NewInsert().Model(p).Exec(ctx)
	return err
}

// UpdateStatus updates only the status + updated_at columns.
func (r *BunRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string, updatedAt time.Time) error {
	_, err := r.db.NewUpdate().
		Model((*Principal)(nil)).
		Set("status = ?", status).
		Set("updated_at = ?", updatedAt).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

// UpdateLock toggles the is_locked column.
func (r *BunRepository) UpdateLock(ctx context.Context, id uuid.UUID, isLocked bool, updatedAt time.Time) error {
	_, err := r.db.NewUpdate().
		Model((*Principal)(nil)).
		Set("is_locked = ?", isLocked).
		Set("updated_at = ?", updatedAt).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

// ListAttributes returns all (key, value) attributes attached to a Principal.
func (r *BunRepository) ListAttributes(ctx context.Context, principalID uuid.UUID) ([]*PrincipalAttribute, error) {
	out := []*PrincipalAttribute{}
	err := r.db.NewSelect().Model(&out).Where("principal_id = ?", principalID).Order("key ASC").Scan(ctx)
	return out, err
}
