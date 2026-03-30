// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package org_units owns the OrgUnit entity — a self-referencing tree
// of organisational nodes. Two trees coexist: the internal hierarchy
// (`is_internal = true`) seeded by deployment / migration; and external
// hierarchies (`is_internal = false`) synced from source systems via
// the REST API. The HTTP surface only writes external nodes.
package org_units //nolint:revive,stylecheck // package name matches the slice path

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// OrgUnit is a node in an org-unit tree.
type OrgUnit struct {
	bun.BaseModel `bun:"table:org_units,alias:ou"`

	ID          uuid.UUID  `bun:"id,pk,type:uuid"     json:"id"`
	ExternalID  string     `bun:"external_id,notnull" json:"external_id"`
	Name        string     `bun:"name,notnull"        json:"name"`
	ParentID    *uuid.UUID `bun:"parent_id"           json:"parent_id,omitempty"`
	Description *string    `bun:"description"         json:"description,omitempty"`
	IsInternal  bool       `bun:"is_internal,notnull" json:"is_internal"`
	CreatedAt   time.Time  `bun:"created_at,notnull"  json:"created_at"`
	UpdatedAt   time.Time  `bun:"updated_at,notnull"  json:"updated_at"`
}
