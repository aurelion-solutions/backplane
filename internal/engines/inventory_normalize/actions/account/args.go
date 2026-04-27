// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package account

// Args is the input contract for the inventory_normalize.account
// action. Fields are populated by the matcher from the
// inventory.ingest.batch_received event via the pipeline's
// args_from_payload mapping.
type Args struct {
	BatchID string `json:"batch_id"`
	Source  string `json:"source"`
	LakeRef string `json:"lake_ref"`
}

// Result is the output contract — counts for observability.
type Result struct {
	Read     int `json:"read"`
	Upserted int `json:"upserted"`
	Skipped  int `json:"skipped"`
}
