// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package compliance_projection

import (
	"time"

	"github.com/google/uuid"
)

// Coverage state values. A control is `covered` only on positive
// evidence (evaluated population, zero violations, zero gaps).
const (
	StateCovered      = "covered"
	StateFailed       = "failed"
	StatePartial      = "partial"
	StateNotEvaluable = "not_evaluable"
)

// Period is the time window the projection covers, sourced from the
// assessment run. AsOf is the point-in-time anchor; Start/End bound a
// period-mode run. All nil for a plain snapshot run.
type Period struct {
	AsOf  *time.Time `json:"as_of,omitempty"`
	Start *time.Time `json:"start,omitempty"`
	End   *time.Time `json:"end,omitempty"`
}

// ControlCoverage is the rolled-up state of one control over a run.
type ControlCoverage struct {
	ControlID  string `json:"control_id"`
	Title      string `json:"title"`
	Category   string `json:"category,omitempty"`
	State      string `json:"state"`
	Violations int    `json:"violations"`
	Gaps       int    `json:"gaps"`
	Evaluated  bool   `json:"evaluated"`
}

// CoverageReport is the per-run, per-projection coverage table.
type CoverageReport struct {
	Projection     string            `json:"projection"`
	Name           string            `json:"name"`
	Type           string            `json:"type,omitempty"`
	CriteriaSource string            `json:"criteria_source,omitempty"`
	Disclaimer     string            `json:"disclaimer,omitempty"`
	AssessmentRun  uuid.UUID         `json:"assessment_run_id"`
	Period         Period            `json:"period"`
	Summary        map[string]int    `json:"summary"`
	Controls       []ControlCoverage `json:"controls"`
}

// ProjectionSummary is one row of the "which projections are available
// for this run" list, with a coverage roll-up.
type ProjectionSummary struct {
	Projection string         `json:"projection"`
	Name       string         `json:"name"`
	Type       string         `json:"type,omitempty"`
	Summary    map[string]int `json:"summary"`
}

// FindingRef is a thin reference to a supporting finding in a control
// detail — enough for the consumer to render and deep-link, not the full
// finding payload.
type FindingRef struct {
	ID         uuid.UUID  `json:"id"`
	Kind       string     `json:"kind"`
	Severity   string     `json:"severity"`
	Status     string     `json:"status"`
	TargetType *string    `json:"target_type,omitempty"`
	TargetID   *uuid.UUID `json:"target_id,omitempty"`
	ScopeValue *string    `json:"scope_value,omitempty"`
}

// GapRef is a thin reference to a not_evaluable outcome (a blind spot)
// that prevents a control from being proven covered.
type GapRef struct {
	RuleID          string   `json:"rule_id"`
	Kind            string   `json:"kind"`
	TargetType      string   `json:"target_type"`
	TargetKey       string   `json:"target_key,omitempty"`
	MissingEvidence []string `json:"missing_evidence,omitempty"`
}

// ControlDetail is the per-control drill-down: state plus the supporting
// findings and the blind spots that hold it back.
type ControlDetail struct {
	Projection    string       `json:"projection"`
	AssessmentRun uuid.UUID    `json:"assessment_run_id"`
	Period        Period       `json:"period"`
	Control       Control      `json:"control"`
	State         string       `json:"state"`
	Violations    []FindingRef `json:"violations"`
	Gaps          []GapRef     `json:"gaps"`
}
