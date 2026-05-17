// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package policies hosts the catalog-sync side of the policy mirror.
//
// Cartridges are the source of truth; the inventory/policies table is
// a projection. This package owns the loop that walks every cartridge
// every few seconds, diffs the live manifest set against the PG
// projection, and applies INSERT / UPDATE / soft-delete / resurrect.
//
// One backplane replica at a time runs the loop, gated by a Postgres
// advisory lock — same pattern as the orchestrator beat. Other
// replicas tick but skip their work; consumer processes (worker, PDP)
// reload from their own per-process mtime watcher independently.
package policies
