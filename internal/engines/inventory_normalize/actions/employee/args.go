// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package employee

// Args is the input contract — populated by the matcher from the
// inventory.ingest.batch_received event via args_from_payload.
type Args struct {
	BatchID string `json:"batch_id"`
	Source  string `json:"source"`
	LakeRef string `json:"lake_ref"`
}

// Result is the output contract — counts for observability.
type Result struct {
	Read                      int `json:"read"`
	Skipped                   int `json:"skipped"`
	PersonsCreated            int `json:"persons_created"`
	PersonsMatched            int `json:"persons_matched"`
	Unresolved                int `json:"unresolved"`
	EmploymentsAdded          int `json:"employments_added"`
	EmploymentsAlreadyMatched int `json:"employments_already_matched"`
	OrgUnitUnresolved         int `json:"org_unit_unresolved"`
}
