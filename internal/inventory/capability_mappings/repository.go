// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package capability_mappings

import (
	"context"

	"github.com/uptrace/bun"
)

// Repository is the persistence boundary for CapabilityMapping rules.
//
// ListActive returns all rules with is_active=TRUE. The projector
// walks every rule for every input grant — the registry is small
// (admin-managed, typically dozens to a few hundred) so an in-memory
// pass is fine; no indexed pre-filter.
type Repository interface {
	ListActive(ctx context.Context, db bun.IDB) ([]*CapabilityMapping, error)
}

// BunRepository is the production Postgres-backed implementation.
type BunRepository struct{}

// NewBunRepository constructs a BunRepository.
func NewBunRepository() *BunRepository {
	return &BunRepository{}
}

// ListActive returns every active mapping, ordered by id for
// determinism in the projection output.
func (r *BunRepository) ListActive(ctx context.Context, db bun.IDB) ([]*CapabilityMapping, error) {
	out := []*CapabilityMapping{}
	err := db.NewSelect().
		Model(&out).
		Where("is_active = TRUE").
		Order("id").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}
