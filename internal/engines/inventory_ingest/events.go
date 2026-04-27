// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package inventory_ingest

const (
	EventActorComponent = "engines.inventory_ingest"

	// EventBatchReceived signals a Process call completed and the
	// resulting changed/new records (if any) landed in the lake.
	// Carries: batch_id, source, dataset_type, lake_ref (nullable
	// when nothing was written), received, written, skipped, new,
	// changed.
	EventBatchReceived = "inventory.ingest.batch_received"
)
