// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package access_generate

import (
	"context"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// resolveRequestedInitiatives projects approved access requests into
// the set of initiatives that *should* exist for the principal right
// now.
//
// NOT IMPLEMENTED — STUB.
//
// Source: the future ITSM Gateway engine. Until that lands the only
// "approved request" state lives in third-party ITSM systems (Jira
// SD, ServiceNow, …) and we have no read-side projection here.
//
// Expected shape, once implemented:
//
//   - Read approved, non-cancelled, non-expired requests for the
//     principal from the ITSM Gateway's read side.
//   - For each request, produce a plannedInitiative with
//     Kind="requested" and Justification carrying:
//     { "request_id": "...", "approver_id": "...", "approved_at": "..." }
//   - SourceRuleID is the request id so the audit trail traces back
//     to the exact ticket.
//   - A request whose planned validity window has not yet started
//     produces an initiative with ValidFrom set into the future;
//     the diff against current still treats it as "planned" and
//     creates the row — only `ListFilter.ActiveOnly` hides it from
//     "in-force" reads.
//   - When a request is cancelled / expired in the source system,
//     the ITSM Gateway publishes a `request.revoked` event that
//     triggers a Recompute for the principal; this resolver then
//     omits the cancelled request from the planned set, and the
//     diff tombstones the matching initiative.
//
// Until the gateway exists, this returns nil — the inheritance pass
// alone drives every Recompute, and requested-kind initiatives
// neither appear in planned nor get tombstoned (they cannot exist
// in the system to begin with).
func (e *Engine) resolveRequestedInitiatives(_ context.Context, _ bun.IDB, _ uuid.UUID, _ RecomputeFilter) ([]plannedInitiative, error) {
	return nil, nil
}
