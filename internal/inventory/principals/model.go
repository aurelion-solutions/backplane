// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package principals owns the Principal polymorphic abstraction over
// Employment, Workload, and Customer "bodies". Each Principal row
// carries a `kind` discriminator plus exactly one principal_*_id
// foreign key pointing at its underlying body.
//
// Principal is the single point in the inventory layer that makes
// access decisions. It carries:
//
//   - kind + principal_*_id  — which body it represents
//   - status                 — lifecycle posture (kind-specific)
//   - is_locked              — operational/admin access lock
//                              (kind-agnostic, the single switch that
//                              revokes access for ANY identity)
//
// Lifecycle status and lock are intentionally separate axes. A
// Principal can be in lifecycle `active` AND `is_locked=true` (admin
// suspended), or in lifecycle `expired` with `is_locked=false`
// (naturally aged out). Access checks AND across both.
package principals

import (
	"time"

	"github.com/aurelion-solutions/backplane/internal/inventory/shared"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Principal is the persistent polymorphic identity row.
type Principal struct {
	bun.BaseModel `bun:"table:principals,alias:p"`

	ID                    uuid.UUID            `bun:"id,pk,type:uuid"            json:"id"`
	ExternalID            string               `bun:"external_id,notnull"        json:"external_id"`
	Kind                  shared.PrincipalKind `bun:"kind,notnull"               json:"kind"`
	PrincipalEmploymentID *uuid.UUID           `bun:"principal_employment_id"    json:"principal_employment_id,omitempty"`
	PrincipalWorkloadID   *uuid.UUID           `bun:"principal_workload_id"      json:"principal_workload_id,omitempty"`
	PrincipalCustomerID   *uuid.UUID           `bun:"principal_customer_id"      json:"principal_customer_id,omitempty"`
	Status                string               `bun:"status,notnull"             json:"status"`
	IsLocked              bool                 `bun:"is_locked,notnull"          json:"is_locked"`
	CreatedAt             time.Time            `bun:"created_at,notnull"         json:"created_at"`
	UpdatedAt             time.Time            `bun:"updated_at,notnull"         json:"updated_at"`
}

// PrincipalAttribute is a single (key, value) tag attached to a
// Principal — cross-body tagging meaningful regardless of the
// underlying body's own attribute set.
type PrincipalAttribute struct {
	bun.BaseModel `bun:"table:principal_attributes,alias:pa"`

	ID          uuid.UUID `bun:"id,pk,type:uuid"      json:"id"`
	PrincipalID uuid.UUID `bun:"principal_id,notnull" json:"principal_id"`
	Key         string    `bun:"key,notnull"          json:"key"`
	Value       string    `bun:"value,notnull"        json:"value"`
}
