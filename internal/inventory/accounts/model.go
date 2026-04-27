// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package accounts

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Account is the persistent record of a provider user-mailbox.
//
// Natural key: (ApplicationID, Username). One row per Application per
// username; if a provider renames a user the row stays in place and
// any downstream FKs remain valid.
//
// ExternalID is the provider's own identifier (AD SID, Okta user id,
// SAP BNAME) — kept for traceability, not for matching. Source is
// the connector instance / provider tag that produced the row.
//
// Attrs holds provider-specific extras the action did not promote to
// dedicated columns (e.g. AD attributes, Okta profile fields).
type Account struct {
	bun.BaseModel `bun:"table:accounts,alias:acc"`

	ID            uuid.UUID      `bun:"id,pk,type:uuid"                json:"id"`
	ApplicationID uuid.UUID      `bun:"application_id,notnull"         json:"application_id"`
	Username      string         `bun:"username,notnull"               json:"username"`
	ExternalID    string         `bun:"external_id,notnull"            json:"external_id"`
	Source        string         `bun:"source,notnull"                 json:"source"`
	DisplayName   *string        `bun:"display_name"                   json:"display_name,omitempty"`
	Email         *string        `bun:"email"                          json:"email,omitempty"`
	IsActive      bool           `bun:"is_active,notnull"              json:"is_active"`
	IsPrivileged  bool           `bun:"is_privileged,notnull"          json:"is_privileged"`
	MFAEnabled    bool           `bun:"mfa_enabled,notnull"            json:"mfa_enabled"`
	Status        *string        `bun:"status"                         json:"status,omitempty"`
	Attrs         map[string]any `bun:"attrs,type:jsonb,notnull"       json:"attrs"`
	CreatedAt     time.Time      `bun:"created_at,notnull"             json:"created_at"`
	UpdatedAt     time.Time      `bun:"updated_at,notnull"             json:"updated_at"`
}
