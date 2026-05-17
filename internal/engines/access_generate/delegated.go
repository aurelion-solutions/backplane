// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package access_generate

import (
	"context"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// resolveDelegatedInitiatives projects active delegations into the
// set of initiatives the principal *should* hold right now as a
// delegate of another principal.
//
// NOT IMPLEMENTED — STUB.
//
// Source: the future ITSM Gateway engine, same as `requested`. A
// delegation differs from a request in that the granting principal
// (the delegator) is a separate identity inside Aurelion, not an
// external approver — the audit story has to capture both subject
// and delegator.
//
// Expected shape, once implemented:
//
//   - Read active delegations naming the subject as the delegate.
//   - For each delegation, produce a plannedInitiative with
//     Kind="delegated" and Justification carrying:
//     { "delegated_by": "<principal_id>", "delegation_id": "..." }
//   - SourceRuleID is the delegation id.
//   - ValidFrom / ValidUntil on the resulting Initiative mirror the
//     delegation's own window — a "delegate for two weeks" delegation
//     produces an initiative with the same window, no separate
//     scheduling state.
//   - When the delegator revokes or the delegation expires, the
//     gateway emits the corresponding Recompute trigger; this
//     resolver then omits the dead delegation and the diff
//     tombstones the matching initiative.
//
// Until the gateway exists, returns nil. No delegated-kind
// initiatives can be planned, so none get created or tombstoned by
// this engine.
func (e *Engine) resolveDelegatedInitiatives(_ context.Context, _ bun.IDB, _ uuid.UUID, _ RecomputeFilter) ([]plannedInitiative, error) {
	return nil, nil
}
