// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package access_grant_record is the inventory_normalize.access_grant_record
// orchestrator action. On every inventory.ingest.batch_received event
// for dataset_type=access_grant_record the matcher fires a one-step
// pipeline whose single step is this action.
//
// What the action does (pure projection — no state outside Postgres):
//
//  1. Reads the batch's lake file (storage.ReadBatch) using the
//     lake_ref carried in the event payload.
//  2. Loads every active CapabilityMapping rule.
//  3. For each grant record:
//     - resolves the Account by (application_id, username);
//     - against every mapping, runs the projector:
//       application_id filter, action_slug filter, XOR resource
//       match, scope_value resolution;
//     - upserts the resulting CapabilityGrant(s) by lineage key
//       (source_grant_external_id, source_capability_mapping_id).
//
// scope_value_source kinds supported now: constant, application_id,
// principal_attribute, resource_external_id. resource_attribute is
// not implemented yet — it requires a lake lookup into AccessArtifact
// records by external_id.
package access_grant_record
