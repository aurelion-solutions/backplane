// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package findings

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Status values for Finding.Status. CHECK constraint on the DB column
// enforces the same closed set.
const (
	StatusOpen         = "open"
	StatusAcknowledged = "acknowledged"
	StatusResolved     = "resolved"
	StatusMitigated    = "mitigated"
)

// Severity values for Finding.Severity. CHECK constraint on the DB
// column enforces the same closed set. Matches the kernel RiskLevel.
const (
	SeverityCritical = "critical"
	SeverityHigh     = "high"
	SeverityMedium   = "medium"
	SeverityLow      = "low"
)

// Convenience constants for finding kinds the platform currently
// recognises. Kind is a free VARCHAR on the DB column — new kinds may
// arrive from new policies without touching this list.
const (
	KindSoD              = "sod"
	KindOrphanAccess     = "orphan_access"
	KindTerminatedAccess = "terminated_access"
	KindUnusedAccess     = "unused_access"
	KindPrivilegedAccess = "privileged_access"
)

// Finding is one row in the findings table — a single detected
// violation or anomaly.
//
// Identity is enforced by the DB unique constraint over
// (kind, principal_id, account_id, policy_id, scope_key_id,
// scope_value, evidence_hash) with NULLS NOT DISTINCT, so the same
// emission on a later assessment run reuses the existing row instead
// of inserting a duplicate.
//
// PrincipalID or AccountID must be non-nil (DB CHECK).
type Finding struct {
	bun.BaseModel `bun:"table:findings,alias:f"`

	ID                          uuid.UUID  `bun:"id,pk,type:uuid"                                json:"id"`
	AssessmentRunID             uuid.UUID  `bun:"assessment_run_id,notnull,type:uuid"            json:"assessment_run_id"`
	Kind                        string     `bun:"kind,notnull"                                   json:"kind"`
	PrincipalID                 *uuid.UUID `bun:"principal_id,type:uuid"                         json:"principal_id,omitempty"`
	AccountID                   *uuid.UUID `bun:"account_id,type:uuid"                           json:"account_id,omitempty"`
	PolicyID                    *uuid.UUID `bun:"policy_id,type:uuid"                            json:"policy_id,omitempty"`
	ScopeKeyID                  *uuid.UUID `bun:"scope_key_id,type:uuid"                         json:"scope_key_id,omitempty"`
	ScopeValue                  *string    `bun:"scope_value"                                    json:"scope_value,omitempty"`
	Severity                    string     `bun:"severity,notnull"                               json:"severity"`
	Status                      string     `bun:"status,notnull"                                 json:"status"`
	MatchedCapabilityGrantIDs   []string   `bun:"matched_capability_grant_ids,type:jsonb,notnull" json:"matched_capability_grant_ids"`
	MatchedEffectiveGrantIDs    []string   `bun:"matched_effective_grant_ids,type:jsonb,notnull"  json:"matched_effective_grant_ids"`
	MatchedAccessFactIDs        []string   `bun:"matched_access_fact_ids,type:jsonb,notnull"      json:"matched_access_fact_ids"`
	EvidenceHash                string     `bun:"evidence_hash,notnull"                          json:"evidence_hash"`
	ActiveMitigationID          *uuid.UUID `bun:"active_mitigation_id,type:uuid"                 json:"active_mitigation_id,omitempty"`
	ProposedMitigationID        *uuid.UUID `bun:"proposed_mitigation_id,type:uuid"               json:"proposed_mitigation_id,omitempty"`
	DetectedAt                  time.Time  `bun:"detected_at,notnull"                            json:"detected_at"`
	EvaluatedAt                 time.Time  `bun:"evaluated_at,notnull"                           json:"evaluated_at"`
	StatusChangedAt             *time.Time `bun:"status_changed_at"                              json:"status_changed_at,omitempty"`
	StatusReason                *string    `bun:"status_reason"                                  json:"status_reason,omitempty"`
}
