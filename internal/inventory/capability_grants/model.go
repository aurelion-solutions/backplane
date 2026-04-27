// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package capability_grants

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// CapabilityGrant is one projected (account, capability, scope) tuple
// with lineage back to the source grant record and the mapping rule.
//
// ScopeValue NULL means the capability is GLOBAL in that scope key
// dimension. TombstonedAt non-NULL means the source grant is no
// longer observed and the grant is logically retired (the row is
// kept for audit).
type CapabilityGrant struct {
	bun.BaseModel `bun:"table:capability_grants,alias:cg"`

	ID                         uuid.UUID  `bun:"id,pk,type:uuid"                         json:"id"`
	AccountID                  uuid.UUID  `bun:"account_id,notnull"                      json:"account_id"`
	CapabilityID               uuid.UUID  `bun:"capability_id,notnull"                   json:"capability_id"`
	ScopeKeyID                 uuid.UUID  `bun:"scope_key_id,notnull"                    json:"scope_key_id"`
	ScopeValue                 *string    `bun:"scope_value"                             json:"scope_value,omitempty"`
	ApplicationID              uuid.UUID  `bun:"application_id,notnull"                  json:"application_id"`
	SourceGrantExternalID      string     `bun:"source_grant_external_id,notnull"        json:"source_grant_external_id"`
	SourceCapabilityMappingID  uuid.UUID  `bun:"source_capability_mapping_id,notnull"    json:"source_capability_mapping_id"`
	ObservedAt                 time.Time  `bun:"observed_at,notnull"                     json:"observed_at"`
	TombstonedAt               *time.Time `bun:"tombstoned_at"                           json:"tombstoned_at,omitempty"`
	CreatedAt                  time.Time  `bun:"created_at,notnull"                      json:"created_at"`
	UpdatedAt                  time.Time  `bun:"updated_at,notnull"                      json:"updated_at"`
}
