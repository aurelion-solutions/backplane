// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package employee_provider_mappings

import (
	"context"

	"github.com/uptrace/bun"
)

// Repository is the persistence boundary for Mapping rules.
type Repository interface {
	ListActiveByProvider(ctx context.Context, db bun.IDB, provider string) ([]*Mapping, error)
}

// BunRepository is the production Postgres-backed implementation.
type BunRepository struct{}

// NewBunRepository constructs a BunRepository.
func NewBunRepository() *BunRepository {
	return &BunRepository{}
}

// ListActiveByProvider returns every active mapping for the given
// provider, ordered by id for deterministic resolver behaviour.
func (r *BunRepository) ListActiveByProvider(ctx context.Context, db bun.IDB, provider string) ([]*Mapping, error) {
	out := []*Mapping{}
	err := db.NewSelect().
		Model(&out).
		Where("provider = ? AND is_active = TRUE", provider).
		Order("id").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return out, nil
}
