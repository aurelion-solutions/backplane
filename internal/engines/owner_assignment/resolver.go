// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package owner_assignment is the minimal owner-resolution engine. A
// finding needs an accountable owner for routing; ownership is carried
// as inventory data on the application row (who owns this governed
// system), not UI-managed config. A finding inherits its account's
// application owner.
//
// The resolver is built once per assessment run from the applications
// table and answers application-id → owner lookups in memory.
package owner_assignment

import (
	"context"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Resolver answers application-id → owner lookups.
type Resolver struct {
	byApp map[uuid.UUID]string
}

// NewResolver wraps a prebuilt application→owner map (used in tests).
func NewResolver(byApp map[uuid.UUID]string) *Resolver {
	return &Resolver{byApp: byApp}
}

// Load builds a Resolver from the applications table. Applications with
// no declared owner are simply absent from the map.
func Load(ctx context.Context, db *bun.DB) (*Resolver, error) {
	var rows []struct {
		ID    uuid.UUID `bun:"id"`
		Owner *string   `bun:"owner"`
	}
	if err := db.NewSelect().
		Table("applications").
		Column("id", "owner").
		Scan(ctx, &rows); err != nil {
		return nil, err
	}
	m := make(map[uuid.UUID]string, len(rows))
	for _, r := range rows {
		if r.Owner != nil && *r.Owner != "" {
			m[r.ID] = *r.Owner
		}
	}
	return &Resolver{byApp: m}, nil
}

// ForApplication returns the owner for an application, or "" when none is
// declared. A nil resolver answers "" so callers need not guard.
func (r *Resolver) ForApplication(id uuid.UUID) string {
	if r == nil {
		return ""
	}
	return r.byApp[id]
}
