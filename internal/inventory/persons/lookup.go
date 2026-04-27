// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package persons

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// AttributeLookup is the read-side helper the employee resolver
// needs: given a (canonical_key, value) pair, find a Person via its
// EAV attributes table.
//
// Separate from Repository so the resolver does not pull in the full
// Person CRUD surface and so the read goes through the action's
// runner transaction (tx bun.IDB), keeping reads consistent with
// pending writes in the same step.
type AttributeLookup interface {
	FindByAttribute(ctx context.Context, tx bun.IDB, key, value string) (uuid.UUID, bool, error)
}

// AttributeLookupBunRepository is the Postgres-backed implementation.
type AttributeLookupBunRepository struct{}

// NewAttributeLookupBunRepository constructs an AttributeLookupBunRepository.
func NewAttributeLookupBunRepository() *AttributeLookupBunRepository {
	return &AttributeLookupBunRepository{}
}

// FindByAttribute returns (personID, true, nil) on hit,
// (uuid.Nil, false, nil) on miss, (_, false, err) on DB failure.
//
// The query uses the (key, value) index in person_attributes for
// constant-time lookup.
func (l *AttributeLookupBunRepository) FindByAttribute(ctx context.Context, tx bun.IDB, key, value string) (uuid.UUID, bool, error) {
	pa := new(PersonAttribute)
	err := tx.NewSelect().
		Model(pa).
		Where("key = ? AND value = ?", key, value).
		Limit(1).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, false, nil
		}
		return uuid.Nil, false, err
	}
	return pa.PersonID, true, nil
}
