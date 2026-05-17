// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package person is the inventory_normalize action that upserts
// Person rows directly into Postgres from a lake batch.
//
// Minimal v1: read the lake batch, pull `external_id` + `full_name`
// from each record, upsert by `external_id`. Provider-specific
// attributes / mappings are not consumed here yet — they belong on
// the employee-normalize path where the employment-side mapping
// catalog already exists.
package person
