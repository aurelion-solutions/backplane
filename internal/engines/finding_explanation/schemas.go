// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package finding_explanation

import (
	"time"

	"github.com/google/uuid"
)

// Generation status. A failed status is a generation failure, never a
// finding failure.
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
)

// Reference kinds — the four anchors a claim may cite. Every Reference
// handed to the model carries one of these.
const (
	RefFinding  = "finding"
	RefEvidence = "evidence"
	RefPolicy   = "policy"
	RefSource   = "source"
)

// Reference is one labelled fact placed in the model's context. The
// Label (e.g. "E1", "P1", "F") is the stable handle the model must use
// to cite it; the validation boundary is exactly the set of Labels
// supplied here.
type Reference struct {
	Label  string `json:"label"`
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Detail string `json:"detail"`
}

// Citation is one validated back-reference from the narrative to an
// input Reference. A Citation only exists in a persisted explanation if
// its Label matched a Reference that was actually in the input.
type Citation struct {
	Label string `json:"label"`
	Kind  string `json:"kind"`
	ID    string `json:"id"`
}

// ExplainRequest is the body of POST /findings/:id/explanations.
//
// Provider optionally overrides which configured backend the gateway
// uses. Force regenerates even when a fresh cached explanation exists.
// Language is a short language code (e.g. "fr", "ru") requesting the
// narrative in that language; unknown/empty falls back to English. It is
// part of the cache key, so each language is generated and cached once.
type ExplainRequest struct {
	Provider string `json:"provider,omitempty"`
	Force    bool   `json:"force,omitempty"`
	Language string `json:"language,omitempty"`
}

// ExplanationView is the outward shape of one explanation artifact.
type ExplanationView struct {
	ID              uuid.UUID  `json:"id"`
	FindingID       uuid.UUID  `json:"finding_id"`
	AssessmentRunID uuid.UUID  `json:"assessment_run_id"`
	Status          string     `json:"status"`
	Narrative       string     `json:"narrative,omitempty"`
	Citations       []Citation `json:"citations"`
	ModelRef        string     `json:"model_ref,omitempty"`
	Error           string     `json:"error,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
}

// view renders an Explanation row as its outward shape.
func (e *Explanation) view() ExplanationView {
	v := ExplanationView{
		ID:              e.ID,
		FindingID:       e.FindingID,
		AssessmentRunID: e.AssessmentRunID,
		Status:          e.Status,
		Narrative:       e.Narrative,
		Citations:       e.Citations,
		ModelRef:        e.ModelRef,
		CreatedAt:       e.CreatedAt,
		CompletedAt:     e.CompletedAt,
	}
	if e.Citations == nil {
		v.Citations = []Citation{}
	}
	if e.Error != nil {
		v.Error = *e.Error
	}
	return v
}
