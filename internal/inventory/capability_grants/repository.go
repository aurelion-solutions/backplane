// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package capability_grants

import (
	"context"

	"github.com/uptrace/bun"
)

// Repository is the persistence boundary for CapabilityGrant rows.
//
// Upsert is idempotent on the lineage key
// (source_grant_external_id, source_capability_mapping_id) — the same
// source grant projected by the same mapping always converges to one
// row.
type Repository interface {
	Upsert(ctx context.Context, tx bun.IDB, g *CapabilityGrant) error
}

// BunRepository is the production Postgres-backed implementation.
type BunRepository struct{}

// NewBunRepository constructs a BunRepository.
func NewBunRepository() *BunRepository {
	return &BunRepository{}
}

// Upsert inserts a new grant or refreshes the existing row keyed by
// the lineage unique index.
func (r *BunRepository) Upsert(ctx context.Context, tx bun.IDB, g *CapabilityGrant) error {
	_, err := tx.NewInsert().
		Model(g).
		On("CONFLICT (source_grant_external_id, source_capability_mapping_id) DO UPDATE").
		Set("account_id      = EXCLUDED.account_id").
		Set("capability_id   = EXCLUDED.capability_id").
		Set("scope_key_id    = EXCLUDED.scope_key_id").
		Set("scope_value     = EXCLUDED.scope_value").
		Set("application_id  = EXCLUDED.application_id").
		Set("observed_at     = EXCLUDED.observed_at").
		Set("tombstoned_at   = NULL").
		Set("updated_at      = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}
