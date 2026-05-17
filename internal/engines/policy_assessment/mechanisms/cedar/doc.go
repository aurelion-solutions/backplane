// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package cedar implements the policy_assessment.Handler for
// mechanism=cedar — Cedar policies (RBAC + ABAC + ReBAC in one
// language) backed by cedar-go.
//
// Author writes Cedar text in a sibling .cedar file; handler loads it
// at Prepare time, compiles it into a cedar PolicySet, and runs
// cedar.Authorize on every Evaluate. Entities + Request context are
// built by the caller (typically the PDP AuthZen transport) and
// supplied through the policy_assessment.Request.Context map.
//
// Expected Context fields (caller contract):
//
//	"principal":  {"type": "User",    "id": "alice"}
//	"action":     {"type": "Action",  "id": "view"}
//	"resource":   {"type": "Photo",   "id": "vacation.jpg"}
//	"context":    map[string]any{...}        // optional, becomes cedar Context Record
//	"entities":   []map[string]any{...}      // optional, additional entity records
package cedar
