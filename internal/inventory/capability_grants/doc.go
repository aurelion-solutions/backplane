// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package capability_grants owns the CapabilityGrant entity — the
// output of inventory_normalize.access_grant_record projection.
//
// Natural key (humans care about this for queries):
//
//	(account_id, capability_id, scope_key_id, scope_value)
//
// Lineage key (idempotency uses this):
//
//	(source_grant_external_id, source_capability_mapping_id)
//
// scope_value NULL ⇒ GLOBAL in that scope dimension. Re-running the
// same projection with the same mapping over the same source grant
// is a no-op via the unique lineage index.
package capability_grants
