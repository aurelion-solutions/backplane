// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package capability_scope_keys

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// CapabilityScopeKey is one scope dimension in the catalog.
//
// Code is the stable identifier referenced by capability_mappings
// and capability_grants. A grant with scope_value NULL means the
// capability applies GLOBAL in this dimension (no narrowing).
type CapabilityScopeKey struct {
	bun.BaseModel `bun:"table:capability_scope_keys,alias:csk"`

	ID          uuid.UUID `bun:"id,pk,type:uuid"          json:"id"`
	Code        string    `bun:"code,notnull"             json:"code"`
	Name        string    `bun:"name,notnull"             json:"name"`
	Description *string   `bun:"description"              json:"description,omitempty"`
	IsActive    bool      `bun:"is_active,notnull"        json:"is_active"`
	CreatedAt   time.Time `bun:"created_at,notnull"       json:"created_at"`
	UpdatedAt   time.Time `bun:"updated_at,notnull"       json:"updated_at"`
}
