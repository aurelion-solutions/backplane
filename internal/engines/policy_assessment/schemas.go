// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policy_assessment

import "time"

// SubjectFacts is the resolved owner-chain / lineage view of a subject.
//
// CONTRACT (F3): `input.subject` (this type) is the resolved lineage
// view — it carries the terminus of the full ownership chain
// (Ownership.Terminus). `input.principal.owner` (PrincipalFacts.Owner)
// is the raw direct-owner reference — a single un-resolved owner ref
// with no lineage. They are DISTINCT views, not duplicates.
//
// The ispm-workload-posture cartridge reads ONLY `input.subject` (never
// `input.principal.owner`). Any future rule author MUST NOT confuse
// these two representations.
type SubjectFacts struct {
	Kind      string          `json:"kind,omitempty"`
	ID        string          `json:"id,omitempty"`
	Ownership *OwnershipFacts `json:"ownership,omitempty"`
}

// OwnershipFacts carries the resolved lineage details for a subject.
type OwnershipFacts struct {
	Terminus            string     `json:"terminus,omitempty"`
	OwnerPersonID       string     `json:"owner_person_id,omitempty"`
	OwnerLabel          string     `json:"owner_label,omitempty"`
	LastTerminationDate *time.Time `json:"last_termination_date,omitempty"`
}

// RuleResult is the unified wrapper for one policy evaluation.
//
// Either or both output variables may be populated, depending on the
// rule class:
//
//   - reactive rules (gates, anomaly findings) → Decision set,
//     ProjectedFacts empty;
//   - generative rules (birthright, leaver, grace) → Decision nil,
//     ProjectedFacts non-empty;
//   - hybrid (rare) → both populated.
//
// Mirrors the kernel `RuleResult` (see kernel
// `engines/policy_assessment/schemas.py` + `RULE_CONTRACT.md`).
type RuleResult struct {
	Decision       *Decision       `json:"decision,omitempty"`
	ProjectedFacts []ProjectedFact `json:"projected_facts,omitempty"`
}

// ---------------------------------------------------------------------
// Output side — Decision / ProjectedFact / Reason
// ---------------------------------------------------------------------

// Effect is the gate verdict — populated only for gate-style reactive
// rules. Anomaly-finding rules leave Effect empty and surface their
// result through RiskLevel + Signals.
const (
	EffectAllow = "allow"
	EffectDeny  = "deny"
)

// RiskLevel classifies the severity / criticality axis. Orthogonal to
// Effect — anomaly findings carry RiskLevel without Effect.
const (
	RiskCritical = "critical"
	RiskHigh     = "high"
	RiskMedium   = "medium"
	RiskLow      = "low"
)

// Initiative tags a projected access fact with its provenance. Used by
// connectors to decide grant / revoke priority and grace-period
// extension semantics.
const (
	InitiativeBirthright = "birthright"
	InitiativeRequested  = "requested"
	InitiativeDelegated  = "delegated"
	InitiativeGrace      = "grace"
)

// Reason is the audit trail for one matched condition path inside a
// rule.
//
// `RuleID` is the cartridge-scoped policy id ("<cartridge>/<rule_id>").
// `MatchedConditions` and `FactValues` together let the consumer
// reproduce which input keys triggered the rule. `Produced` carries the
// rule-specific output payload (severity bumps, projected attributes,
// etc.).
type Reason struct {
	RuleID            string         `json:"rule_id"`
	RuleKind          string         `json:"rule_kind,omitempty"`
	Precedence        int            `json:"precedence,omitempty"`
	MatchedConditions map[string]any `json:"matched_conditions,omitempty"`
	FactValues        map[string]any `json:"fact_values,omitempty"`
	Produced          map[string]any `json:"produced,omitempty"`
}

// Decision is the reactive verdict on a (principal, target) pair.
//
// Effect is populated only for gate-style rules. Anomaly-finding rules
// leave Effect empty and surface their result through RiskLevel +
// Signals. There are no `actions` here — the model is declarative;
// connectors derive grant / revoke / update from the diff between
// CurrentFacts and ProjectedFacts.
//
// Signals is intentionally polymorphic — each entry is either a plain
// string marker ("orphaned_account_recent_login") or a structured
// finding object (`{"kind": "sod_conflict", ...}`). Consumers (UI,
// findings) interpret the dict shape via its `kind` field. Mirrors the
// kernel `Signal = str | dict[str, Any]` union.
type Decision struct {
	Effect    string   `json:"effect,omitempty"`
	RiskLevel string   `json:"risk_level,omitempty"`
	Signals   []any    `json:"signals,omitempty"`
	Reasons   []Reason `json:"reasons,omitempty"`
}

// DesiredState is the declarative target state for a projected fact.
// Present=true means "this access must exist"; Present=false means
// "this access must NOT exist" (revoke if present).
type DesiredState struct {
	Present    bool           `json:"present"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// ProjectedFact is the desired access fact emitted by a generative
// rule. The connector picks up the diff between CurrentFacts and
// ProjectedFacts and applies the smallest change set that converges on
// DesiredState.
type ProjectedFact struct {
	Target       TargetFacts  `json:"target"`
	Initiative   string       `json:"initiative"`
	ValidFrom    *time.Time   `json:"valid_from,omitempty"`
	ValidUntil   *time.Time   `json:"valid_until,omitempty"`
	DesiredState DesiredState `json:"desired_state"`
	RiskLevel    string       `json:"risk_level,omitempty"`
	Signals      []any        `json:"signals,omitempty"`
	Reasons      []Reason     `json:"reasons,omitempty"`
}

// ---------------------------------------------------------------------
// Input side — Facts
// ---------------------------------------------------------------------

// Facts is the canonical PDP input. The runtime caller (PDP transport,
// policy-assessment action) populates the sections relevant to its evaluation —
// rules read only what they care about.
//
// `Now` is mandatory. Every other section is optional.
type Facts struct {
	Principal          *PrincipalFacts        `json:"principal,omitempty"`
	Target             *TargetFacts           `json:"target,omitempty"`
	Action             string                 `json:"action,omitempty"`
	Resource           *Resource              `json:"resource,omitempty"`
	Context            *ContextFacts          `json:"context,omitempty"`
	PrincipalContext   *PrincipalContextFacts `json:"principal_context,omitempty"`
	CurrentFacts       []map[string]any       `json:"current_facts,omitempty"`
	CurrentInitiatives []map[string]any       `json:"current_initiatives,omitempty"`
	Threat             *ThreatFacts           `json:"threat,omitempty"`
	Entities           []EntityRecord         `json:"entities,omitempty"`
	Now                time.Time              `json:"now"`
	// Extra carries caller-supplied facts the typed sections above do
	// not cover (transport-specific request fields, custom signals).
	// Mechanism handlers may read it but should treat it as
	// untyped / best-effort.
	Extra map[string]any `json:"extra,omitempty"`
	// EvidencePresent records which truth-input keys the caller could
	// supply evidence for (e.g. "mfa_evidence": true). The stack_check
	// gate consults it to decide not_evaluable. A missing or false key
	// means the evidence is absent. Set by the caller (assess action).
	EvidencePresent map[string]bool `json:"evidence_present,omitempty"`
	// Subject carries the resolved owner-chain / lineage view for workload
	// evaluations. Nil in the account path (omitempty marshals it away;
	// account-path OPA policies are unaffected). Populated only in the
	// workload assessment pass via factsForWorkload.
	Subject *SubjectFacts `json:"subject,omitempty"`
}

// PrincipalFacts is the principal snapshot. May be nil for
// orphan-account-style rules where no principal is known.
type PrincipalFacts struct {
	ID                  string         `json:"id"`
	Kind                string         `json:"kind,omitempty"`
	Status              string         `json:"status,omitempty"`
	OrgUnit             string         `json:"org_unit,omitempty"`
	StartDate           *time.Time     `json:"start_date,omitempty"`
	TermDate            *time.Time     `json:"term_date,omitempty"`
	WorkloadKind        string         `json:"workload_kind,omitempty"`
	Owner               *OwnerFacts    `json:"owner,omitempty"`
	ExpiresAt           *time.Time     `json:"expires_at,omitempty"`
	EmailVerified       *bool          `json:"email_verified,omitempty"`
	TenantID            string         `json:"tenant_id,omitempty"`
	TenantRole          string         `json:"tenant_role,omitempty"`
	TenantStatus        string         `json:"tenant_status,omitempty"`
	PlanTier            string         `json:"plan_tier,omitempty"`
	RequiredConsentsMet *bool          `json:"required_consents_met,omitempty"`
	MFAEnabled          *bool          `json:"mfa_enabled,omitempty"`
	CapabilitySlugs     []string       `json:"capability_slugs,omitempty"`
	Attributes          map[string]any `json:"attributes,omitempty"`
}

// OwnerFacts is a workload subject's owner reference.
type OwnerFacts struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// TargetFacts is the object access is granted to. Most fields are
// optional; each rule picks only what it needs.
type TargetFacts struct {
	Application          string           `json:"application,omitempty"`
	Kind                 string           `json:"kind,omitempty"` // "account", "channel_membership", "role", …
	ResourceType         string           `json:"resource_type,omitempty"`
	Resource             string           `json:"resource,omitempty"`
	ID                   string           `json:"id,omitempty"`
	PrincipalID          string           `json:"principal_id,omitempty"`
	AccountStatus        string           `json:"account_status,omitempty"`
	AccountIsPrivileged  *bool            `json:"account_is_privileged,omitempty"`
	AccountMFAEnabled    *bool            `json:"account_mfa_enabled,omitempty"`
	LastLoginAt          *time.Time       `json:"last_login_at,omitempty"`
	Initiatives          []InitiativeFact `json:"initiatives,omitempty"`
	PrivilegeLevel       string           `json:"privilege_level,omitempty"`
	Environment          string           `json:"environment,omitempty"`
	DataSensitivity      string           `json:"data_sensitivity,omitempty"`
	HasPendingAttest     bool             `json:"has_pending_attestation,omitempty"`
	PendingReattestation bool             `json:"pending_reattestation,omitempty"`
}

// InitiativeFact is one initiative already attached to a target.
type InitiativeFact struct {
	Type       string     `json:"type"`
	Origin     string     `json:"origin,omitempty"`
	ValidFrom  *time.Time `json:"valid_from,omitempty"`
	ValidUntil *time.Time `json:"valid_until,omitempty"`
}

// ContextFacts is the session / environment context for gate-style
// rules. Transport / country / IP plus open-ended extras.
type ContextFacts struct {
	Transport string         `json:"transport,omitempty"`
	Country   string         `json:"country,omitempty"`
	IP        string         `json:"ip,omitempty"`
	Extra     map[string]any `json:"extra,omitempty"`
}

// PrincipalContextFacts is the extended principal context — used by
// generative (joiner / leaver) rules to read attributes that decide
// what birthright access to project.
type PrincipalContextFacts struct {
	OrgUnitID  string         `json:"org_unit_id,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// ThreatFacts carries risk signals for risk-aware rules.
type ThreatFacts struct {
	RiskScore             *float64 `json:"risk_score,omitempty"`
	ActiveIndicators      []string `json:"active_indicators,omitempty"`
	DaysSinceLastLogin    *int     `json:"days_since_last_login,omitempty"`
	DaysSinceLastUse      *int     `json:"days_since_last_use,omitempty"`
	FailedAuthCount       *int     `json:"failed_auth_count,omitempty"`
	CredentialCompromised *bool    `json:"credential_compromised,omitempty"`
	UEBARiskScore         *float64 `json:"ueba_risk_score,omitempty"`
	BehavioralAnomaly     *bool    `json:"behavioral_anomaly,omitempty"`
}

// Resource is the object of the access decision when the rule needs to
// carry more than what TargetFacts.Application + .Resource encode
// (typical for AuthZ over arbitrary REST resources).
type Resource struct {
	Type       string         `json:"type"`
	ID         string         `json:"id"`
	Properties map[string]any `json:"properties,omitempty"`
}

// EntityRecord is the input shape consumed by entity-graph mechanisms
// (Cedar, graph_analysis). Maps directly to Cedar entity JSON:
// `{uid, attrs, parents}`.
type EntityRecord struct {
	UID     EntityUID      `json:"uid"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Parents []EntityUID    `json:"parents,omitempty"`
}

// EntityUID is the type + id pair identifying one entity in the graph.
type EntityUID struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}
