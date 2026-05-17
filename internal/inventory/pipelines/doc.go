// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package pipelines is the inventory slice for the PG mirror of
// cartridge-defined pipeline definitions.
//
// Cartridges are the source of truth; this table is a projection
// rebuilt by core/pipelines' sync loop. The YAML body is NOT mirrored
// — only metadata + a content hash (used by the sync loop to detect
// changes). The orchestrator's pipeline catalog still loads
// definitions directly from cartridge files at runtime.
//
// The REST surface is read-only. Edits land via cartridge changes
// picked up by the next sync tick.
package pipelines
