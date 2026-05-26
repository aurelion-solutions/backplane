// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package applications owns the Application domain entity — the catalog
// of external systems Aurelion governs. An Application points to one or
// more connector instances via tag-set matching (required_connector_tags
// ⊆ connector_instance.tags). Persistence is bun on Postgres; the HTTP
// surface lives in routes.go.
package applications

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Application is the persistent catalog entry for an external system.
//
// The wire shape (snake_case JSON tags) is locked: aurelion-engineering-
// studio and other clients pin to these field names. Changing them is a
// breaking change for every API consumer.
type Application struct {
	bun.BaseModel `bun:"table:applications,alias:a"`

	ID                    uuid.UUID      `bun:"id,pk,type:uuid"            json:"id"`
	Name                  string         `bun:"name,notnull"               json:"name"`
	Code                  string         `bun:"code,notnull"               json:"code"`
	Config                map[string]any `bun:"config,type:jsonb,notnull"  json:"config"`
	RequiredConnectorTags []string       `bun:"required_connector_tags,type:jsonb,notnull" json:"required_connector_tags"`
	IsActive              bool           `bun:"is_active,notnull"          json:"is_active"`
	// Owner is the accountable party for this governed system (an email
	// or team handle), carried as inventory data. Findings on this
	// application's accounts inherit it for routing. Nullable — not
	// every application has a declared owner yet.
	Owner     *string   `bun:"owner"                      json:"owner,omitempty"`
	CreatedAt time.Time `bun:"created_at,notnull"         json:"created_at"`
	UpdatedAt time.Time `bun:"updated_at,notnull"         json:"updated_at"`
}
