// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package orgunit is the inventory_normalize.orgunit orchestrator
// action. On every inventory.ingest.batch_received event for
// dataset_type=orgunit the matcher fires a one-step pipeline whose
// single step is this action.
//
// One lake record carries one subtree (root-first). The action walks
// the tree top-down and upserts every node into org_units by
// external_id, threading parent_id from the resolved ancestor.
//
// Out of scope (deferred — no schema for them yet):
//   - manager_id (Person identifier — would need org_unit_attributes
//     or a typed FK after the account → principal matcher lands),
//   - label, company, meta (would need EAV/JSONB sidecar).
package orgunit
