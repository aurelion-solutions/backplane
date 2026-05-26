// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policy_evaluation_outcomes

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Outcome values for PolicyEvaluationOutcome.Outcome. The DB CHECK
// constraint enforces the same closed set.
const (
	OutcomeMatched      = "matched"
	OutcomeNotMatched   = "not_matched"
	OutcomeNotEvaluable = "not_evaluable"
)

// TargetType values for PolicyEvaluationOutcome.TargetType. The DB
// CHECK constraint enforces the same closed set. account/subject/workload
// are concrete entities (TargetRef set); source/pipeline are aggregate
// targets for coverage gaps (TargetRef nil, identity via TargetKey).
const (
	TargetAccount  = "account"
	TargetSubject  = "subject"
	TargetWorkload = "workload"
	TargetSource   = "source"
	TargetPipeline = "pipeline"
)

// PolicyEvaluationOutcome is one row in the policy_evaluation_outcomes
// table — the ternary result of evaluating one policy against one
// target during one assessment run.
//
// MissingEvidence is empty for matched / not_matched and lists the
// absent truth-input keys for not_evaluable (the biconditional).
type PolicyEvaluationOutcome struct {
	bun.BaseModel `bun:"table:policy_evaluation_outcomes,alias:peo"`

	ID              uuid.UUID  `bun:"id,pk,type:uuid"                       json:"id"`
	AssessmentRunID uuid.UUID  `bun:"assessment_run_id,notnull,type:uuid"   json:"assessment_run_id"`
	CartridgeID     string     `bun:"cartridge_id,notnull"                  json:"cartridge_id"`
	RuleID          string     `bun:"rule_id,notnull"                       json:"rule_id"`
	TargetType      string     `bun:"target_type,notnull"                   json:"target_type"`
	TargetRef       *uuid.UUID `bun:"target_ref,type:uuid"                  json:"target_ref,omitempty"`
	TargetKey       string     `bun:"target_key,notnull"                    json:"target_key"`
	Outcome         string     `bun:"outcome,notnull"                       json:"outcome"`
	MissingEvidence []string   `bun:"missing_evidence,type:jsonb,notnull"   json:"missing_evidence"`
	SourceID        *uuid.UUID `bun:"source_id,type:uuid"                   json:"source_id,omitempty"`
	EvaluatedAt     time.Time  `bun:"evaluated_at,notnull"                  json:"evaluated_at"`
}
