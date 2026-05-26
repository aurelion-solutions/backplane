// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package evidence_chain persists append-only lineage records linking an
// outcome or finding back through the truth stack (raw row → normalised
// fact → effective grant → justifying initiative) to the policy that
// produced it.
//
// Records are immutable: never updated in place, and RecordChain is
// idempotent on chain_hash (a deterministic SHA-256 over the component
// ids). Re-recording the same chain returns the existing row. Every
// chain is anchored to a scan run, giving evidence an immutable
// timestamped reference — the temporal foundation period queries
// (Slice 5 / M2) build on without retrofitting the shape.
package evidence_chain
