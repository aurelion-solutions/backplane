// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package access_profile

import (
	"time"

	"github.com/google/uuid"
)

// GrantView is one observed (capability, scope) the account holds.
// ScopeValue nil means the capability is GLOBAL in that scope dimension.
type GrantView struct {
	CapabilitySlug string  `json:"capability_slug"`
	CapabilityName string  `json:"capability_name"`
	ScopeKeyCode   string  `json:"scope_key_code"`
	ScopeValue     *string `json:"scope_value,omitempty"`
}

// AccountView is one account the person holds in an application, with
// its observed spec (privilege, MFA, effective state) and its grants.
type AccountView struct {
	ID             uuid.UUID   `json:"id"`
	Username       string      `json:"username"`
	DisplayName    *string     `json:"display_name,omitempty"`
	IsActive       bool        `json:"is_active"`
	IsPrivileged   bool        `json:"is_privileged"`
	MFAEnabled     bool        `json:"mfa_enabled"`
	EffectiveState string      `json:"effective_state"`
	Grants         []GrantView `json:"grants"`
}

// InitiativeView is one justification authorising access in an
// application. CapabilityName nil = an account-level initiative ("needs
// an account here"); non-nil = grant-level. Expired is computed against
// ValidUntil at read time.
type InitiativeView struct {
	ID             uuid.UUID  `json:"id"`
	Kind           string     `json:"kind"`
	CapabilityName *string    `json:"capability_name,omitempty"`
	Actor          string     `json:"actor"`
	ValidFrom      time.Time  `json:"valid_from"`
	ValidUntil     *time.Time `json:"valid_until,omitempty"`
	Expired        bool       `json:"expired"`
}

// ApplicationView groups everything the person has in one application:
// the accounts they hold and the initiatives that justify access there.
type ApplicationView struct {
	ApplicationID   uuid.UUID        `json:"application_id"`
	ApplicationName string           `json:"application_name"`
	ApplicationCode string           `json:"application_code"`
	Accounts        []AccountView    `json:"accounts"`
	Initiatives     []InitiativeView `json:"initiatives"`
}

// EmploymentView is one working period. Active is computed against the
// date window at read time (end_date NULL or in the future).
type EmploymentView struct {
	ID        uuid.UUID  `json:"id"`
	Code      string     `json:"code"`
	StartDate time.Time  `json:"start_date"`
	EndDate   *time.Time `json:"end_date,omitempty"`
	Active    bool       `json:"active"`
}

// AccessProfile is the full nested document returned for one person.
// Terminated is true when the person holds no active employment.
type AccessProfile struct {
	PersonID     uuid.UUID         `json:"person_id"`
	ExternalID   string            `json:"external_id"`
	FullName     string            `json:"full_name"`
	Terminated   bool              `json:"terminated"`
	Employments  []EmploymentView  `json:"employments"`
	Applications []ApplicationView `json:"applications"`
}
