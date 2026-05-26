// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package account is the inventory_normalize.account orchestrator
// action. On every inventory.ingest.batch_received event for
// dataset_type=account the matcher fires a one-step pipeline whose
// single step is this action.
//
// What the action does:
//
//  1. Reads the batch's lake file (storage.ReadBatch) using the
//     lake_ref carried in the event payload.
//  2. For each record validates required payload fields
//     (application_id, username).
//  3. Upserts the corresponding row in accounts keyed by
//     (application_id, username) — the inventory natural key.
//
// What it does NOT do:
//
//   - match the account to a principal (employee / workload / customer) —
//     that is a separate downstream engine,
//   - infer fields the connector did not send,
//   - reconcile across applications.
package account
