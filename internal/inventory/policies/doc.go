// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package policies is the inventory slice for the PG mirror of
// cartridge-defined policy rules.
//
// Cartridges are the source of truth; this table is a projection
// rebuilt by core/policies' sync loop. Rego bodies are NOT mirrored
// here — only metadata (rule_id, cartridge_ref, mechanism, severity,
// version, meta, lifecycle timestamps).
//
// The REST surface is read-only. Edits land via cartridge changes
// picked up by the next sync tick.
package policies
