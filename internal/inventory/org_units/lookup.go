// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package org_units

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Lookup is the narrow read-side helper used by downstream normalize
// actions (employee, in particular) to resolve a contract-level
// `org_unit_id` / `org_unit_name` pair into a Postgres `org_units.id`.
//
// Two lookup paths:
//   - GetIDByExternalID — the canonical, stable path.
//   - GetIDByDisplayName — fallback when only the human-readable
//     name is delivered by the source. First match wins.
//
// Both return (uuid.Nil, false, nil) on a miss rather than an error,
// so the caller can decide whether to drop the link or fail.
type Lookup interface {
	GetIDByExternalID(ctx context.Context, tx bun.IDB, externalID string) (uuid.UUID, bool, error)
	GetIDByDisplayName(ctx context.Context, tx bun.IDB, displayName string) (uuid.UUID, bool, error)
}

// LookupBunRepository is the Postgres-backed Lookup implementation.
type LookupBunRepository struct{}

// NewLookupBunRepository constructs a LookupBunRepository.
func NewLookupBunRepository() *LookupBunRepository {
	return &LookupBunRepository{}
}

// GetIDByExternalID returns (id, true, nil) on hit, (uuid.Nil, false, nil)
// on miss, (_, false, err) on DB failure.
func (l *LookupBunRepository) GetIDByExternalID(ctx context.Context, tx bun.IDB, externalID string) (uuid.UUID, bool, error) {
	return scanOneID(ctx, tx, "external_id = ?", externalID)
}

// GetIDByDisplayName returns the first OrgUnit whose display_name
// matches exactly. Display names are not unique by schema — the first
// hit wins; document this in the calling action.
func (l *LookupBunRepository) GetIDByDisplayName(ctx context.Context, tx bun.IDB, displayName string) (uuid.UUID, bool, error) {
	return scanOneID(ctx, tx, "display_name = ?", displayName)
}

func scanOneID(ctx context.Context, tx bun.IDB, whereClause string, arg any) (uuid.UUID, bool, error) {
	var id uuid.UUID
	err := tx.NewSelect().
		Model((*OrgUnit)(nil)).
		Column("id").
		Where(whereClause, arg).
		Limit(1).
		Scan(ctx, &id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, false, nil
		}
		return uuid.Nil, false, err
	}
	return id, true, nil
}
