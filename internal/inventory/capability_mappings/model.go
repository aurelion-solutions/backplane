// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package capability_mappings

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// CapabilityMapping is one rule in the projection catalog.
//
// Resource selector is XOR over ResourceID / ResourceKind /
// ResourcePathGlob — exactly one is non-nil; enforced by a DB CHECK
// constraint. ApplicationID and ActionSlug are optional filters.
type CapabilityMapping struct {
	bun.BaseModel `bun:"table:capability_mappings,alias:cm"`

	ID                uuid.UUID      `bun:"id,pk,type:uuid"             json:"id"`
	CapabilityID      uuid.UUID      `bun:"capability_id,notnull"       json:"capability_id"`
	ApplicationID     *uuid.UUID     `bun:"application_id"              json:"application_id,omitempty"`
	ResourceID        *uuid.UUID     `bun:"resource_id"                 json:"resource_id,omitempty"`
	ResourceKind      *string        `bun:"resource_kind"               json:"resource_kind,omitempty"`
	ResourcePathGlob  *string        `bun:"resource_path_glob"          json:"resource_path_glob,omitempty"`
	ActionSlug        *string        `bun:"action_slug"                 json:"action_slug,omitempty"`
	ScopeKeyID        uuid.UUID      `bun:"scope_key_id,notnull"        json:"scope_key_id"`
	ScopeValueSource  map[string]any `bun:"scope_value_source,type:jsonb,notnull" json:"scope_value_source"`
	IsActive          bool           `bun:"is_active,notnull"           json:"is_active"`
	CreatedAt         time.Time      `bun:"created_at,notnull"          json:"created_at"`
	UpdatedAt         time.Time      `bun:"updated_at,notnull"          json:"updated_at"`
}
