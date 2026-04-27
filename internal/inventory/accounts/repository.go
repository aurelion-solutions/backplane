// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package accounts

import (
	"context"

	"github.com/uptrace/bun"
)

// Repository is the persistence boundary for Account aggregates.
//
// Upsert runs against the supplied bun.IDB — caller controls the
// transaction. This lets pipeline actions piggy-back on the
// orchestrator's per-step Tx without managing one of their own.
type Repository interface {
	Upsert(ctx context.Context, tx bun.IDB, a *Account) error
}

// BunRepository is the production Postgres-backed implementation.
type BunRepository struct{}

// NewBunRepository constructs a BunRepository.
func NewBunRepository() *BunRepository {
	return &BunRepository{}
}

// Upsert inserts a new Account or updates the existing one keyed by
// (application_id, username). updated_at is always refreshed.
func (r *BunRepository) Upsert(ctx context.Context, tx bun.IDB, a *Account) error {
	_, err := tx.NewInsert().
		Model(a).
		On("CONFLICT (application_id, username) DO UPDATE").
		Set("external_id   = EXCLUDED.external_id").
		Set("source        = EXCLUDED.source").
		Set("display_name  = EXCLUDED.display_name").
		Set("email         = EXCLUDED.email").
		Set("is_active     = EXCLUDED.is_active").
		Set("is_privileged = EXCLUDED.is_privileged").
		Set("mfa_enabled   = EXCLUDED.mfa_enabled").
		Set("status        = EXCLUDED.status").
		Set("attrs         = EXCLUDED.attrs").
		Set("updated_at    = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}
