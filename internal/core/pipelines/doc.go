// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package pipelines hosts the catalog-sync side of the pipeline mirror.
//
// Cartridges are the source of truth; the inventory/pipelines table
// is a projection. This package owns the loop that walks every
// cartridge every few seconds, diffs the live definition set against
// the PG projection, and applies INSERT / UPDATE / soft-delete /
// resurrect.
//
// One backplane replica at a time runs the loop, gated by a Postgres
// advisory lock — same pattern as core/policies and orchestrator/beat.
package pipelines
