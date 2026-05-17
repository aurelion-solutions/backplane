// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package run

// Args is the input contract for the access_generate.run action.
//
// PrincipalID is required — every Recompute call is scoped to a
// single principal. ApplicationID and CapabilityID are optional
// filters that narrow what the engine touches; both empty means
// "rebuild the whole (principal, ∀ applications) scope".
type Args struct {
	PrincipalID   string `json:"principal_id"`
	ApplicationID string `json:"application_id,omitempty"`
	CapabilityID  string `json:"capability_id,omitempty"`
}

// Result reports counters from the Recompute pass.
type Result struct {
	CreatedCount    int `json:"created_count"`
	TombstonedCount int `json:"tombstoned_count"`
	EventsEmitted   int `json:"events_emitted"`
}
