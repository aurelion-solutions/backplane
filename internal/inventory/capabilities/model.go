// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package capabilities

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Capability is one business-meaningful action in the catalog.
//
// Slug is the stable identifier referenced by capability_mappings;
// name and description are human-facing.
type Capability struct {
	bun.BaseModel `bun:"table:capabilities,alias:cap"`

	ID          uuid.UUID `bun:"id,pk,type:uuid"          json:"id"`
	Slug        string    `bun:"slug,notnull"             json:"slug"`
	Name        string    `bun:"name,notnull"             json:"name"`
	Description *string   `bun:"description"              json:"description,omitempty"`
	IsActive    bool      `bun:"is_active,notnull"        json:"is_active"`
	CreatedAt   time.Time `bun:"created_at,notnull"       json:"created_at"`
	UpdatedAt   time.Time `bun:"updated_at,notnull"       json:"updated_at"`
}
