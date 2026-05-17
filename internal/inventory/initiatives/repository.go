// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package initiatives

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Repository is the persistence boundary for Initiative aggregates.
//
// Every method takes a `bun.IDB` so callers (typically a pipeline
// action) can piggy-back on the orchestrator's per-step transaction.
//
// The interface deliberately has no `Delete` method — initiatives
// are audit records and are tombstoned via `Tombstone`, never
// removed.
type Repository interface {
	Create(ctx context.Context, tx bun.IDB, i *Initiative) error
	Tombstone(ctx context.Context, tx bun.IDB, id uuid.UUID) error
	GetByID(ctx context.Context, tx bun.IDB, id uuid.UUID) (*Initiative, error)
	List(ctx context.Context, tx bun.IDB, f ListFilter) ([]*Initiative, int, error)
}

// ListFilter narrows what List returns. Zero value lists every row.
//
// PrincipalID / ApplicationID / CapabilityID restrict by exact id.
// AccountInits restricts to rows where capability_id IS NULL;
// GrantInits restricts to rows where capability_id IS NOT NULL —
// these two flags are mutually exclusive and ignored when both are
// false. ActiveOnly returns rows currently in force: not tombstoned
// AND valid_from <= NOW() AND (valid_until IS NULL OR valid_until >
// NOW()). TombstonedOnly returns rows with tombstoned_at set,
// regardless of validity window. The two flags are mutually
// exclusive. Kind filters by the kind column. Limit/Offset paginate;
// default ordering is created_at DESC so a scroll over the audit
// trail starts with the most recent events.
type ListFilter struct {
	PrincipalID    *uuid.UUID
	ApplicationID  *uuid.UUID
	CapabilityID   *uuid.UUID
	AccountInits   bool
	GrantInits     bool
	ActiveOnly     bool
	TombstonedOnly bool
	Kind           string
	Limit          int
	Offset         int
}

// BunRepository is the production Postgres-backed implementation.
type BunRepository struct{}

// NewBunRepository constructs a BunRepository.
func NewBunRepository() *BunRepository { return &BunRepository{} }

// Create inserts a new Initiative. Multiple active initiatives for
// the same target are allowed (no partial unique index in the
// schema) — duplicate justifications are normal.
//
// `created_at` and `valid_from` default to time.Now() when zero so
// call sites do not have to supply them. A future-dated initiative
// is created by setting `valid_from` explicitly.
func (r *BunRepository) Create(ctx context.Context, tx bun.IDB, i *Initiative) error {
	now := time.Now()
	if i.CreatedAt.IsZero() {
		i.CreatedAt = now
	}
	if i.ValidFrom.IsZero() {
		i.ValidFrom = now
	}
	_, err := tx.NewInsert().Model(i).Exec(ctx)
	return err
}

// Tombstone marks the initiative inactive by stamping `tombstoned_at`.
//
// Idempotent: calling Tombstone on an already-tombstoned row is a
// no-op (the WHERE filters out tombstoned rows and we treat zero
// rows affected as success when the id exists). Returns ErrNotFound
// only when the id does not exist at all.
func (r *BunRepository) Tombstone(ctx context.Context, tx bun.IDB, id uuid.UUID) error {
	res, err := tx.NewUpdate().
		Model((*Initiative)(nil)).
		Set("tombstoned_at = NOW()").
		Where("id = ?", id).
		Where("tombstoned_at IS NULL").
		Exec(ctx)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 1 {
		return nil
	}
	// Zero rows: either id is missing or already tombstoned. The
	// second case is a no-op; only the former needs surfacing.
	probe := new(Initiative)
	err = tx.NewSelect().Model(probe).Column("id").Where("id = ?", id).Limit(1).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// GetByID fetches one initiative. Returns ErrNotFound when no row
// carries the given id.
func (r *BunRepository) GetByID(ctx context.Context, tx bun.IDB, id uuid.UUID) (*Initiative, error) {
	out := new(Initiative)
	err := tx.NewSelect().Model(out).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

// List returns a paginated snapshot plus total row count honouring
// the filter. Default ordering is created_at DESC.
func (r *BunRepository) List(ctx context.Context, tx bun.IDB, f ListFilter) ([]*Initiative, int, error) {
	out := []*Initiative{}
	q := tx.NewSelect().Model(&out)

	if f.PrincipalID != nil {
		q = q.Where("principal_id = ?", *f.PrincipalID)
	}
	if f.ApplicationID != nil {
		q = q.Where("application_id = ?", *f.ApplicationID)
	}
	if f.CapabilityID != nil {
		q = q.Where("capability_id = ?", *f.CapabilityID)
	}
	if f.AccountInits && !f.GrantInits {
		q = q.Where("capability_id IS NULL")
	}
	if f.GrantInits && !f.AccountInits {
		q = q.Where("capability_id IS NOT NULL")
	}
	if f.ActiveOnly && !f.TombstonedOnly {
		q = q.Where("tombstoned_at IS NULL").
			Where("valid_from <= NOW()").
			Where("valid_until IS NULL OR valid_until > NOW()")
	}
	if f.TombstonedOnly && !f.ActiveOnly {
		q = q.Where("tombstoned_at IS NOT NULL")
	}
	if f.Kind != "" {
		q = q.Where("kind = ?", f.Kind)
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
