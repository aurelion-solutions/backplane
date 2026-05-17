// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package initiatives

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Initiative is the persistent justification record behind a desired
// account or grant.
//
// Target shape:
//
//   - CapabilityID == nil → account-initiative ("this principal needs
//     an account in this application")
//   - CapabilityID != nil → grant-initiative ("this principal needs
//     this capability on this account")
//
// Multiple active initiatives may coexist for the same target —
// access ⇐ any single active justification.
//
// Lifecycle: rows are created when the source (generative observer,
// human via source-system) issues the justification. They are
// *never* deleted — closure manifests as TombstonedAt being set.
// Closure ≠ revoke: tombstoning an initiative removes the
// justification record; the downstream desired_state recompute then
// decides whether access actually goes away.
type Initiative struct {
	bun.BaseModel `bun:"table:initiatives,alias:init"`

	ID            uuid.UUID      `bun:"id,pk,type:uuid"                  json:"id"`
	PrincipalID   uuid.UUID      `bun:"principal_id,notnull"             json:"principal_id"`
	ApplicationID uuid.UUID      `bun:"application_id,notnull"           json:"application_id"`
	CapabilityID  *uuid.UUID     `bun:"capability_id"                    json:"capability_id,omitempty"`
	Kind          string         `bun:"kind,notnull"                     json:"kind"`
	Justification map[string]any `bun:"justification,type:jsonb,notnull" json:"justification"`
	Actor         string         `bun:"actor,notnull"                    json:"actor"`
	CreatedAt     time.Time      `bun:"created_at,notnull"               json:"created_at"`
	ValidFrom     time.Time      `bun:"valid_from,notnull"               json:"valid_from"`
	ValidUntil    *time.Time     `bun:"valid_until"                      json:"valid_until,omitempty"`
	TombstonedAt  *time.Time     `bun:"tombstoned_at"                    json:"tombstoned_at,omitempty"`
}

// IsActiveAt returns true when the initiative is in force at the
// given moment: not tombstoned, validity window has opened, and has
// not closed (open-ended if ValidUntil is nil). Tombstone overrides
// any validity window — tombstoned_at means "withdrawn", regardless
// of the planned window.
func (i *Initiative) IsActiveAt(t time.Time) bool {
	if i.TombstonedAt != nil {
		return false
	}
	if t.Before(i.ValidFrom) {
		return false
	}
	if i.ValidUntil != nil && !t.Before(*i.ValidUntil) {
		return false
	}
	return true
}

// IsActive is the IsActiveAt(time.Now()) shortcut.
func (i *Initiative) IsActive() bool { return i.IsActiveAt(time.Now()) }

// Initiative kinds. The value goes into the `kind` column verbatim.
//
//   - KindInheritance — derived from a structural attachment of the
//     principal: org-unit membership today, project membership later.
//     The specific source (which OU, which project) lives in
//     `justification`.
//   - KindRequested — created through an approval workflow at the
//     principal's request.
//   - KindDelegated — issued by another principal acting on the
//     subject's behalf. The delegator lives in `justification`.
//
// Grace-period extensions are not a separate kind — they manifest
// as a follow-up initiative (of the original kind) with a bounded
// ValidUntil, or as ValidUntil being pushed out on an existing row.
const (
	KindInheritance = "inheritance"
	KindRequested   = "requested"
	KindDelegated   = "delegated"
)
