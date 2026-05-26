// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package employee is the inventory_normalize.employee orchestrator
// action. On every inventory.ingest.batch_received event for
// dataset_type=employee the matcher fires a one-step pipeline whose
// single step is this action.
//
// What the action does (one record at a time):
//
//  1. Reads the batch's lake file (storage.ReadBatch) using the
//     lake_ref carried in the event payload.
//  2. Loads active employee_provider_mappings WHERE provider=source.
//  3. Skips records already processed (lookup employment_record_matches
//     by (source, external_id)).
//  4. Resolves the Person:
//     - direct determinator: for each is_determinator=TRUE mapping,
//     find a Person via persons.AttributeLookup.FindByAttribute.
//     On miss + a primary provider, CREATE a Person with the
//     first determinator value as initial attribute.
//     - upstream: for each allow_upstream=TRUE mapping, find a Person
//     by (person_key, value). Returns the first hit.
//     - if neither path lands a Person → unresolved (counter only).
//  5. Upserts all non-determinator mapped attributes into
//     person_attributes.
//  6. Selects or creates one Employment for the Person:
//     - primary provider: SELECT by (person_id, code, start_date);
//     create with defaults if missing.
//     - secondary provider: SELECT any Employment for person_id;
//     skip the record if none exists yet (wait for primary).
//  7. Inserts one employment_record_matches row.
//
// Writes go through ctx.Tx so the whole step is one transaction.
package employee
