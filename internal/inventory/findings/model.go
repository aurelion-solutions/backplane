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
	// KindEvidenceGap is the derived finding for a not_evaluable
	// outcome — a Blind Spot where a required truth input was absent.
	KindEvidenceGap = "evidence_gap"
	// KindWorkloadOwnedByTerminated is fired when a workload's full
	// ownership chain resolves to a terminated human (all employments ended).
	KindWorkloadOwnedByTerminated = "workload_owned_by_terminated"
)

// Target artifact types — the discriminator for a finding's target_id.
// A finding's target is the concrete artifact the problem sits on; the
// identity it concerns lives on the separate principal_id axis.
const (
	TargetAccount              = "account"
	TargetWorkload             = "workload"
	TargetSecretPlain          = "secret_plain"
	TargetSecretCertificate    = "secret_certificate"
	TargetConsentedApplication = "consented_application"
	TargetConsentGrant         = "consent_grant"
)

// PriorityFactor is one named contribution to a finding's priority
// score. The risk engine produces the decomposition; it is stored
// verbatim so the assessment UI can justify a finding's priority
// without recomputation.
type PriorityFactor struct {
	Name   string `json:"name"`
	Points int    `json:"points"`
}

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

	ID uuid.UUID `bun:"id,pk,type:uuid"                                json:"id"`
	// AssessmentRunID is the run that first created the finding (the
	// moment of detection); LastSeenRunID is the most recent run that
	// re-confirmed it. They diverge once a finding survives a re-run:
	// idempotency reuses the existing row, advancing LastSeenRunID while
	// AssessmentRunID stays pinned to first detection. Current-posture
	// views filter on LastSeenRunID ("still present in the latest run").
	AssessmentRunID uuid.UUID `bun:"assessment_run_id,notnull,type:uuid"            json:"assessment_run_id"`
	LastSeenRunID   uuid.UUID `bun:"last_seen_run_id,notnull,type:uuid"             json:"last_seen_run_id"`
	Kind            string    `bun:"kind,notnull"                                   json:"kind"`
	// PrincipalID is the identity axis — the principal the finding
	// concerns (an account's owner, or a workload's own principal).
	PrincipalID *uuid.UUID `bun:"principal_id,type:uuid"                         json:"principal_id,omitempty"`
	// TargetType + TargetID are the artifact axis — the concrete thing
	// the problem sits on. TargetID is a polymorphic reference (no FK)
	// resolved against the table named by TargetType (one of the
	// Target* constants).
	TargetType                *string    `bun:"target_type"                                   json:"target_type,omitempty"`
	TargetID                  *uuid.UUID `bun:"target_id,type:uuid"                            json:"target_id,omitempty"`
	PolicyID                  *uuid.UUID `bun:"policy_id,type:uuid"                            json:"policy_id,omitempty"`
	ScopeKeyID                *uuid.UUID `bun:"scope_key_id,type:uuid"                         json:"scope_key_id,omitempty"`
	ScopeValue                *string    `bun:"scope_value"                                    json:"scope_value,omitempty"`
	Severity                  string     `bun:"severity,notnull"                               json:"severity"`
	Status                    string     `bun:"status,notnull"                                 json:"status"`
	MatchedCapabilityGrantIDs []string   `bun:"matched_capability_grant_ids,type:jsonb,notnull" json:"matched_capability_grant_ids"`
	MatchedEffectiveGrantIDs  []string   `bun:"matched_effective_grant_ids,type:jsonb,notnull"  json:"matched_effective_grant_ids"`
	MatchedAccessFactIDs      []string   `bun:"matched_access_fact_ids,type:jsonb,notnull"      json:"matched_access_fact_ids"`
	EvidenceHash              string     `bun:"evidence_hash,notnull"                          json:"evidence_hash"`
	// Triage denormalisation, stamped at assess time so a finding is
	// self-describing for filtering and grouping without joins:
	// application/source/cartridge it came from, the routed owner
	// (inherited from the application), and a factor-decomposed
	// priority score.
	ApplicationID   *uuid.UUID       `bun:"application_id,type:uuid"            json:"application_id,omitempty"`
	Source          *string          `bun:"source"                             json:"source,omitempty"`
	CartridgeRef    *string          `bun:"cartridge_ref"                      json:"cartridge_ref,omitempty"`
	OwnerRef        *string          `bun:"owner_ref"                          json:"owner_ref,omitempty"`
	PriorityScore   int              `bun:"priority_score,notnull"             json:"priority_score"`
	PriorityFactors []PriorityFactor `bun:"priority_factors,type:jsonb,notnull" json:"priority_factors"`
	// Suggested action + remediation guidance, denormalised from the
	// cartridge finding metadata so the UI shows a cartridge-sourced
	// next step without a separate remediation capability.
	RecommendedAction    *string    `bun:"recommended_action"                 json:"recommended_action,omitempty"`
	Remediation          *string    `bun:"remediation"                        json:"remediation,omitempty"`
	ActiveMitigationID   *uuid.UUID `bun:"active_mitigation_id,type:uuid"                 json:"active_mitigation_id,omitempty"`
	ProposedMitigationID *uuid.UUID `bun:"proposed_mitigation_id,type:uuid"               json:"proposed_mitigation_id,omitempty"`
	DetectedAt           time.Time  `bun:"detected_at,notnull"                            json:"detected_at"`
	EvaluatedAt          time.Time  `bun:"evaluated_at,notnull"                           json:"evaluated_at"`
	StatusChangedAt      *time.Time `bun:"status_changed_at"                              json:"status_changed_at,omitempty"`
	StatusReason         *string    `bun:"status_reason"                                  json:"status_reason,omitempty"`
}
