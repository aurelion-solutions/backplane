// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package access_generate computes the set of initiatives a
// principal *should* hold right now by projecting structural state
// (employment, OU), ITSM requests, and delegations through cartridge
// rules.
//
// Generative is reactive: every Recompute call sees a snapshot of
// the world and produces the diff against the initiatives currently
// recorded for the principal. It does not subscribe to events
// itself — callers (Journey pipelines via the orchestrator action,
// beat-scheduled passes, ad-hoc REST triggers) translate their
// events into Recompute calls.
//
// Output side: writes `initiatives` (create / tombstone), updates
// `desired_state` on affected accounts/grants, and emits
// `inventory.initiative.created` / `inventory.initiative.tombstoned`
// MQ events after the transaction commits.
//
// What this package does NOT write:
//
//   - `validated_state` — that's the PDP validator's column
//     (future `access_validate` engine).
//   - `effective_state` — that's `inventory_normalize` (from
//     connector data) and the future `access_promote` engine (when
//     it ships a command and parks effective at `pending`).
package access_generate
