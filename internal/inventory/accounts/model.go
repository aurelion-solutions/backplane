// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package accounts

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Canonical account state values. Mirrors the value axis of every app
// cartridge's account.yaml state machine. Per-app cartridges may
// introduce additional values, but these five are baked into the
// CHECK constraint and the index targets at the storage level.
const (
	StateNotExist = "not_exist"
	StatePending  = "pending"
	StateBlocked  = "blocked"
	StateInvited  = "invited"
	StateActive   = "active"
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
//
// DesiredState / ValidatedState / EffectiveState carry the three
// independent state observations:
//
//   - EffectiveState: written by inventory_normalize from connector
//     data — what the provider actually has.
//   - DesiredState: written by policy_assessment.generative — what the
//     business policies say the account ought to be.
//   - ValidatedState: written by the PDP validator after generative —
//     the desired state filtered through guardrails (deny grants,
//     SoD, segregation rules).
//
// access_apply reacts to ValidatedState ≠ EffectiveState. Each column
// is owned by exactly one writer; the repository's Set*State methods
// enforce that single-writer contract.
type Account struct {
	bun.BaseModel `bun:"table:accounts,alias:acc"`

	ID            uuid.UUID `bun:"id,pk,type:uuid"           json:"id"`
	ApplicationID uuid.UUID `bun:"application_id,notnull"    json:"application_id"`
	// PrincipalID is the account-assignment edge: which governed
	// identity holds this mailbox. NULL = unassigned (an orphan
	// account). Mirrors workloads.owner_employment_id on the human side.
	PrincipalID  *uuid.UUID `bun:"principal_id,type:uuid"   json:"principal_id,omitempty"`
	Username     string     `bun:"username,notnull"          json:"username"`
	ExternalID   string     `bun:"external_id,notnull"       json:"external_id"`
	Source       string     `bun:"source,notnull"            json:"source"`
	DisplayName  *string    `bun:"display_name"              json:"display_name,omitempty"`
	Email        *string    `bun:"email"                     json:"email,omitempty"`
	IsActive     bool       `bun:"is_active,notnull"         json:"is_active"`
	IsPrivileged bool       `bun:"is_privileged,notnull"     json:"is_privileged"`
	MFAEnabled   bool       `bun:"mfa_enabled,notnull"       json:"mfa_enabled"`
	// Evidence-presence signals. NULL = the source never evidenced this
	// datum ⇒ checks depending on it are not_evaluable (a Blind Spot),
	// rather than silently treating absence as a clean pass.
	MFAEvidenceAt      *time.Time     `bun:"mfa_evidence_at"           json:"mfa_evidence_at,omitempty"`
	OwnerEvidenceAt    *time.Time     `bun:"owner_evidence_at"         json:"owner_evidence_at,omitempty"`
	LastUsedEvidenceAt *time.Time     `bun:"last_used_evidence_at"     json:"last_used_evidence_at,omitempty"`
	Status             *string        `bun:"status"                    json:"status,omitempty"`
	DesiredState       string         `bun:"desired_state,notnull"     json:"desired_state"`
	ValidatedState     string         `bun:"validated_state,notnull"   json:"validated_state"`
	EffectiveState     string         `bun:"effective_state,notnull"   json:"effective_state"`
	Attrs              map[string]any `bun:"attrs,type:jsonb,notnull"  json:"attrs"`
	CreatedAt          time.Time      `bun:"created_at,notnull"        json:"created_at"`
	UpdatedAt          time.Time      `bun:"updated_at,notnull"        json:"updated_at"`
}
