// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policies

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// ListFilter narrows what List returns.
//
// Zero value lists all active rows. CartridgeRef restricts to one
// cartridge namespace. Mechanism restricts to a single mechanism (open
// allowlist filter lives one layer up — engines filter their own
// in-memory registry).
//
// IncludeInactive flips the default which hides soft-deleted rows.
type ListFilter struct {
	CartridgeRef    string
	Mechanism       string
	IncludeInactive bool
	Limit           int
	Offset          int
}

// Repository is the persistence boundary for Policy rows.
//
// The sync side (Upsert, MarkRemoved, Resurrect) is what the
// core/policies sync manager calls every tick. The read side is what
// the REST handlers and consumer engines call.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*Policy, error)
	GetByNaturalKey(ctx context.Context, cartridgeRef, ruleID string) (*Policy, error)
	List(ctx context.Context, f ListFilter) ([]*Policy, int, error)
	ListActiveByCartridge(ctx context.Context, cartridgeRef string) ([]*Policy, error)
	ListActiveByMechanisms(ctx context.Context, mechanisms []string) ([]*Policy, error)

	Upsert(ctx context.Context, p *Policy) error
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
func (r *BunRepository) GetByID(ctx context.Context, id uuid.UUID) (*Policy, error) {
	p := new(Policy)
	err := r.db.NewSelect().Model(p).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return p, nil
}

// GetByNaturalKey returns one row by (cartridge_ref, rule_id).
func (r *BunRepository) GetByNaturalKey(ctx context.Context, cartridgeRef, ruleID string) (*Policy, error) {
	p := new(Policy)
	err := r.db.NewSelect().
		Model(p).
		Where("cartridge_ref = ?", cartridgeRef).
		Where("rule_id = ?", ruleID).
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
func (r *BunRepository) List(ctx context.Context, f ListFilter) ([]*Policy, int, error) {
	out := []*Policy{}
	q := r.db.NewSelect().Model(&out)
	if f.CartridgeRef != "" {
		q = q.Where("cartridge_ref = ?", f.CartridgeRef)
	}
	if f.Mechanism != "" {
		q = q.Where("mechanism = ?", f.Mechanism)
	}
	if !f.IncludeInactive {
		q = q.Where("is_active = TRUE")
	}
	q = q.Order("cartridge_ref ASC", "rule_id ASC")
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

// ListActiveByCartridge returns every active rule belonging to one
// cartridge — the sync manager uses this to diff against the
// cartridge's current manifest set.
func (r *BunRepository) ListActiveByCartridge(ctx context.Context, cartridgeRef string) ([]*Policy, error) {
	out := []*Policy{}
	err := r.db.NewSelect().
		Model(&out).
		Where("cartridge_ref = ?", cartridgeRef).
		Where("is_active = TRUE").
		Order("rule_id ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListActiveByMechanisms returns every active rule whose mechanism is
// in the allowlist. Consumers (PDP, scan engine) filter the catalog
// down to what they know how to evaluate.
func (r *BunRepository) ListActiveByMechanisms(ctx context.Context, mechanisms []string) ([]*Policy, error) {
	if len(mechanisms) == 0 {
		return []*Policy{}, nil
	}
	out := []*Policy{}
	err := r.db.NewSelect().
		Model(&out).
		Where("mechanism IN (?)", bun.In(mechanisms)).
		Where("is_active = TRUE").
		Order("cartridge_ref ASC", "rule_id ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Upsert inserts a new row or updates the existing one keyed by
// (cartridge_ref, rule_id). Always sets is_active=TRUE and clears
// removed_at — used by the sync loop both for fresh inserts and for
// resurrecting previously soft-deleted rows.
func (r *BunRepository) Upsert(ctx context.Context, p *Policy) error {
	_, err := r.db.NewInsert().
		Model(p).
		On("CONFLICT (cartridge_ref, rule_id) DO UPDATE").
		Set("name        = EXCLUDED.name").
		Set("description = EXCLUDED.description").
		Set("mechanism   = EXCLUDED.mechanism").
		Set("severity    = EXCLUDED.severity").
		Set("owner_team  = EXCLUDED.owner_team").
		Set("tags        = EXCLUDED.tags").
		Set("version     = EXCLUDED.version").
		Set("is_active   = TRUE").
		Set("removed_at  = NULL").
		Set("meta        = EXCLUDED.meta").
		Set("updated_at  = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

// MarkRemoved flips an existing row to is_active=false and stamps
// removed_at. The row's metadata stays — findings keep something to
// reference.
func (r *BunRepository) MarkRemoved(ctx context.Context, id uuid.UUID, removedAt time.Time) error {
	res, err := r.db.NewUpdate().
		Model((*Policy)(nil)).
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

// Resurrect flips a soft-deleted row back to active. Used when a
// cartridge brings back a rule the sync loop had previously retired.
// Upsert can do the same as a single statement; this exists for
// callers that already hold the id.
func (r *BunRepository) Resurrect(ctx context.Context, id uuid.UUID, now time.Time) error {
	res, err := r.db.NewUpdate().
		Model((*Policy)(nil)).
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
