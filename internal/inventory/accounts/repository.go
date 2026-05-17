// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package accounts

import (
	"context"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Repository is the persistence boundary for Account aggregates.
//
// Upsert runs against the supplied bun.IDB — caller controls the
// transaction. This lets pipeline actions piggy-back on the
// orchestrator's per-step Tx without managing one of their own.
//
// Each of the three state columns has a dedicated setter
// (SetDesiredState, SetValidatedState, SetEffectiveState). Upsert
// only writes EffectiveState on conflict — connector data tells us
// what is, not what should be — so DesiredState and ValidatedState
// stay under the control of their owning observers
// (policy_assessment.generative and the PDP validator).
//
// List exposes a paginated read path keyed off the supplied bun.IDB.
// Callers (the policy-assessment action, access_apply, future
// read-side surfaces) snapshot the account population without
// locking it.
type Repository interface {
	Upsert(ctx context.Context, tx bun.IDB, a *Account) error
	SetDesiredState(ctx context.Context, tx bun.IDB, id uuid.UUID, state string) error
	SetValidatedState(ctx context.Context, tx bun.IDB, id uuid.UUID, state string) error
	SetEffectiveState(ctx context.Context, tx bun.IDB, id uuid.UUID, state string) error
	List(ctx context.Context, tx bun.IDB, f ListFilter) ([]*Account, int, error)
}

// ListFilter narrows what List returns. Zero value lists every row.
//
// ApplicationID restricts to one application. ActiveOnly drops
// inactive rows (legacy is_active boolean). DesiredState /
// ValidatedState / EffectiveState filter by exact state value when
// non-empty. NeedsApply, when true, returns rows where
// ValidatedState ≠ EffectiveState — the working set of the future
// access_apply engine. Limit and Offset paginate; default ordering
// is created_at ASC so a snapshot pass walks the population
// deterministically.
type ListFilter struct {
	ApplicationID  *uuid.UUID
	ActiveOnly     bool
	DesiredState   string
	ValidatedState string
	EffectiveState string
	NeedsApply     bool
	Limit          int
	Offset         int
}

// BunRepository is the production Postgres-backed implementation.
type BunRepository struct{}

// NewBunRepository constructs a BunRepository.
func NewBunRepository() *BunRepository {
	return &BunRepository{}
}

// Upsert inserts a new Account or updates the existing one keyed by
// (application_id, username). updated_at is always refreshed.
//
// On conflict, only the EffectiveState column among the three state
// columns is overwritten — the connector reports what *is*. The
// DesiredState and ValidatedState columns remain owned by their
// respective observers and are left untouched here.
//
// When a state column is left empty on the input, it falls back to
// StatePending so the row satisfies the CHECK constraint on insert.
func (r *BunRepository) Upsert(ctx context.Context, tx bun.IDB, a *Account) error {
	if a.DesiredState == "" {
		a.DesiredState = StatePending
	}
	if a.ValidatedState == "" {
		a.ValidatedState = StatePending
	}
	if a.EffectiveState == "" {
		a.EffectiveState = StatePending
	}
	_, err := tx.NewInsert().
		Model(a).
		On("CONFLICT (application_id, username) DO UPDATE").
		Set("external_id     = EXCLUDED.external_id").
		Set("source          = EXCLUDED.source").
		Set("display_name    = EXCLUDED.display_name").
		Set("email           = EXCLUDED.email").
		Set("is_active       = EXCLUDED.is_active").
		Set("is_privileged   = EXCLUDED.is_privileged").
		Set("mfa_enabled     = EXCLUDED.mfa_enabled").
		Set("status          = EXCLUDED.status").
		Set("effective_state = EXCLUDED.effective_state").
		Set("attrs           = EXCLUDED.attrs").
		Set("updated_at      = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

// SetDesiredState writes the desired_state column for one account
// and refreshes updated_at. Intended writer:
// policy_assessment.generative.
func (r *BunRepository) SetDesiredState(ctx context.Context, tx bun.IDB, id uuid.UUID, state string) error {
	return setStateColumn(ctx, tx, id, "desired_state", state)
}

// SetValidatedState writes the validated_state column for one
// account and refreshes updated_at. Intended writer: the PDP
// validator.
func (r *BunRepository) SetValidatedState(ctx context.Context, tx bun.IDB, id uuid.UUID, state string) error {
	return setStateColumn(ctx, tx, id, "validated_state", state)
}

// SetEffectiveState writes the effective_state column for one
// account and refreshes updated_at. Intended writers:
// inventory_normalize (when ingest tells us what actually exists in
// the provider) and access_apply (when it ships a command to the
// connector and sets effective back to "pending" until ingest
// confirms).
func (r *BunRepository) SetEffectiveState(ctx context.Context, tx bun.IDB, id uuid.UUID, state string) error {
	return setStateColumn(ctx, tx, id, "effective_state", state)
}

// setStateColumn is the column-agnostic body shared by the three
// state setters. Column is interpolated as a literal so the bun
// query stays parametric for the values that matter (state, id).
func setStateColumn(ctx context.Context, tx bun.IDB, id uuid.UUID, column, state string) error {
	res, err := tx.NewUpdate().
		Model((*Account)(nil)).
		Set(column+" = ?", state).
		Set("updated_at = NOW()").
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

// List returns a paginated snapshot plus total row count honouring the
// filter. Default ordering is created_at ASC so a scan walks the
// population deterministically.
func (r *BunRepository) List(ctx context.Context, tx bun.IDB, f ListFilter) ([]*Account, int, error) {
	out := []*Account{}
	q := tx.NewSelect().Model(&out)
	if f.ApplicationID != nil {
		q = q.Where("application_id = ?", *f.ApplicationID)
	}
	if f.ActiveOnly {
		q = q.Where("is_active = TRUE")
	}
	if f.DesiredState != "" {
		q = q.Where("desired_state = ?", f.DesiredState)
	}
	if f.ValidatedState != "" {
		q = q.Where("validated_state = ?", f.ValidatedState)
	}
	if f.EffectiveState != "" {
		q = q.Where("effective_state = ?", f.EffectiveState)
	}
	if f.NeedsApply {
		q = q.Where("validated_state <> effective_state")
	}
	q = q.Order("created_at ASC")
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
