// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package initiatives is the persistence boundary for the audit
// trail behind every desired-state decision Aurelion makes.
//
// An Initiative answers: "why does this principal need this account
// (or this capability) in this application?". When
// policy_assessment.generative writes `desired_state` on an account
// or grant, it also opens the matching Initiative that carries the
// justification (birthright from org_unit, role assignment, manual
// request, …) and the actor who issued it.
//
// Initiatives are never deleted. Revoke / closure / "justification
// lost" all manifest as tombstone columns being set
// (`closed_at`, `closed_by`, `closure_reason`). The active set is
// "rows WHERE closed_at IS NULL". Partial unique indexes enforce
// at-most-one active initiative per target.
package initiatives
