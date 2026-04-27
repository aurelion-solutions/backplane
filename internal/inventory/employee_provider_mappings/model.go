// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package employee_provider_mappings

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Mapping is one rule for one (provider, record_key) pair.
type Mapping struct {
	bun.BaseModel `bun:"table:employee_provider_mappings,alias:epm"`

	ID             uuid.UUID `bun:"id,pk,type:uuid"             json:"id"`
	Provider       string    `bun:"provider,notnull"            json:"provider"`
	RecordKey      string    `bun:"record_key,notnull"          json:"record_key"`
	PersonKey      string    `bun:"person_key,notnull"          json:"person_key"`
	IsDeterminator bool      `bun:"is_determinator,notnull"     json:"is_determinator"`
	AllowUpstream  bool      `bun:"allow_upstream,notnull"      json:"allow_upstream"`
	IsActive       bool      `bun:"is_active,notnull"           json:"is_active"`
	CreatedAt      time.Time `bun:"created_at,notnull"          json:"created_at"`
	UpdatedAt      time.Time `bun:"updated_at,notnull"          json:"updated_at"`
}
