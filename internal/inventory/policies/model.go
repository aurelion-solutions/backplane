// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policies

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Policy is the persistent record of one cartridge-defined rule.
//
// Natural key: (CartridgeRef, RuleID). One row per (cartridge, rule);
// soft-deleted rows keep that uniqueness so a later resurrection of
// the same rule updates the existing row in place.
//
// Mechanism is a free-form string naming a class-of-evaluation
// handler (e.g. "generic" for plain Rego, "sod" for the DB-backed
// SoD evaluator, "llm_classification" / "risk_scoring" / etc.).
type Policy struct {
	bun.BaseModel `bun:"table:policies,alias:pol"`

	ID           uuid.UUID      `bun:"id,pk,type:uuid"           json:"id"`
	CartridgeRef string         `bun:"cartridge_ref,notnull"     json:"cartridge_ref"`
	RuleID       string         `bun:"rule_id,notnull"           json:"rule_id"`
	Name         string         `bun:"name,notnull"              json:"name"`
	Description  *string        `bun:"description"               json:"description,omitempty"`
	Mechanism    string         `bun:"mechanism,notnull"         json:"mechanism"`
	Severity     *string        `bun:"severity"                  json:"severity,omitempty"`
	OwnerTeam    *string        `bun:"owner_team"                json:"owner_team,omitempty"`
	Tags         []string       `bun:"tags,array,notnull"        json:"tags"`
	Version      int            `bun:"version,notnull"           json:"version"`
	IsActive     bool           `bun:"is_active,notnull"         json:"is_active"`
	RemovedAt    *time.Time     `bun:"removed_at"                json:"removed_at,omitempty"`
	Meta         map[string]any `bun:"meta,type:jsonb,notnull"   json:"meta"`
	CreatedAt    time.Time      `bun:"created_at,notnull"        json:"created_at"`
	UpdatedAt    time.Time      `bun:"updated_at,notnull"        json:"updated_at"`
}
