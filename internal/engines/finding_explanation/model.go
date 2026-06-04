// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package finding_explanation

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Explanation is one generated narrative over a finding — a persisted,
// cited artifact, not a view. It is keyed for caching by
// (finding_id, input_hash): the same finding / evidence / policy /
// template / model does not regenerate.
type Explanation struct {
	bun.BaseModel `bun:"table:finding_explanations,alias:fe"`

	ID uuid.UUID `bun:"id,pk,type:uuid" json:"id"`
	// FindingID is the finding this narrative explains. AssessmentRunID
	// is denormalised from the finding for run-scoped lookups and cache
	// invalidation; PolicyID likewise records which policy produced it.
	FindingID       uuid.UUID  `bun:"finding_id,notnull,type:uuid"        json:"finding_id"`
	AssessmentRunID uuid.UUID  `bun:"assessment_run_id,notnull,type:uuid" json:"assessment_run_id"`
	PolicyID        *uuid.UUID `bun:"policy_id,type:uuid"                  json:"policy_id,omitempty"`
	// InputHash is the cache key: a digest over the rendered prompt
	// inputs (finding + evidence refs + policy + template version +
	// model). A change in any input yields a new hash, invalidating the
	// cached explanation.
	InputHash             string `bun:"input_hash,notnull"              json:"input_hash"`
	ModelRef              string `bun:"model_ref,notnull"               json:"model_ref"`
	PromptTemplateVersion string `bun:"prompt_template_version,notnull" json:"prompt_template_version"`
	Status                string `bun:"status,notnull"                  json:"status"`
	// Narrative is the generated prose (empty until completed).
	Narrative string `bun:"narrative,notnull,default:''" json:"narrative"`
	// Citations are the validated back-references from the narrative to
	// the input refs. Refs is the exact set of labelled facts placed in
	// the model's context — the validation boundary a citation must fall
	// within.
	Citations   []Citation  `bun:"citations,type:jsonb,notnull"    json:"citations"`
	Refs        []Reference `bun:"refs,type:jsonb,notnull"         json:"refs"`
	Error       *string     `bun:"error"                           json:"error,omitempty"`
	CreatedAt   time.Time   `bun:"created_at,notnull"              json:"created_at"`
	CompletedAt *time.Time  `bun:"completed_at"                    json:"completed_at,omitempty"`
}
