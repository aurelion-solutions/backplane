// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package orgunit

// Args is the input contract — populated by the matcher from the
// inventory.ingest.batch_received event via args_from_payload.
type Args struct {
	BatchID string `json:"batch_id"`
	Source  string `json:"source"`
	LakeRef string `json:"lake_ref"`
}

// Result is the output contract — counts for observability.
type Result struct {
	Read     int `json:"read"`     // number of root subtrees in the batch
	Upserted int `json:"upserted"` // total nodes upserted across all subtrees
	Skipped  int `json:"skipped"`  // malformed records / nodes
}
