// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policy_assessment_runs

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Status values for AssessmentRun.Status. The CHECK constraint on the
// DB column enforces the same closed set.
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// Trigger values for AssessmentRun.TriggeredBy. The CHECK constraint
// on the DB column enforces the same closed set.
const (
	TriggerManual   = "manual"
	TriggerAPI      = "api"
	TriggerSchedule = "schedule"
)

// AssessmentRun is one row in the policy_assessment_runs table — a
// single pass over the policy catalogue that evaluates a set of
// objects (principals, accounts) against applicable policies.
//
// Scope is narrowed via the optional ScopePrincipalID /
// ScopeApplicationID FKs: both nil = full-catalogue pass,
// principal-only = per-principal pass, application-only =
// per-application pass.
//
// Counters (FindingsTotal, FindingsBySeverity, FindingsCreatedCount,
// FindingsReusedCount) summarise what the pass produced — the
// authoritative records live in the findings table.
type AssessmentRun struct {
	bun.BaseModel `bun:"table:policy_assessment_runs,alias:par"`

	ID                   uuid.UUID      `bun:"id,pk,type:uuid"                          json:"id"`
	Status               string         `bun:"status,notnull"                           json:"status"`
	TriggeredBy          string         `bun:"triggered_by,notnull"                     json:"triggered_by"`
	StartedAt            *time.Time     `bun:"started_at"                               json:"started_at,omitempty"`
	CompletedAt          *time.Time     `bun:"completed_at"                             json:"completed_at,omitempty"`
	ScopePrincipalID     *uuid.UUID     `bun:"scope_principal_id,type:uuid"             json:"scope_principal_id,omitempty"`
	ScopeApplicationID   *uuid.UUID     `bun:"scope_application_id,type:uuid"           json:"scope_application_id,omitempty"`
	FindingsTotal        int            `bun:"findings_total,notnull"                   json:"findings_total"`
	FindingsBySeverity   map[string]int `bun:"findings_by_severity,type:jsonb,notnull"  json:"findings_by_severity"`
	FindingsCreatedCount int            `bun:"findings_created_count,notnull"           json:"findings_created_count"`
	FindingsReusedCount  int            `bun:"findings_reused_count,notnull"            json:"findings_reused_count"`
	ErrorMessage         *string        `bun:"error_message"                            json:"error_message,omitempty"`
	CreatedAt            time.Time      `bun:"created_at,notnull"                       json:"created_at"`
	CreatedBy            *string        `bun:"created_by"                               json:"created_by,omitempty"`

	// Temporal anchors (Slice 1). AsOf is the point in time the run
	// reflects; Period* stay nil until the Slice 5 period query uses
	// them. OutcomesByKind tallies the ternary outcomes
	// (matched / not_matched / not_evaluable) for the health header.
	AsOf           *time.Time     `bun:"as_of"                       json:"as_of,omitempty"`
	PeriodStart    *time.Time     `bun:"period_start"                json:"period_start,omitempty"`
	PeriodEnd      *time.Time     `bun:"period_end"                  json:"period_end,omitempty"`
	OutcomesByKind map[string]int `bun:"outcomes_by_kind,type:jsonb" json:"outcomes_by_kind,omitempty"`
}
