// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package customers owns the Customer entity — an end-user principal
// independent of Employees and Workloads. Customers are tenant-scoped
// (optional tenant_id) and carry billing-plan + MFA flags. Principal
// status is derived elsewhere (principals slice); customers signals a
// recompute when locked / verified state shifts.
package customers

import (
	"time"

	"github.com/aurelion-solutions/backplane/internal/inventory/shared"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Customer is the persistent end-user principal row.
type Customer struct {
	bun.BaseModel `bun:"table:customers,alias:c"`

	ID            uuid.UUID                  `bun:"id,pk,type:uuid"        json:"id"`
	ExternalID    string                     `bun:"external_id,notnull"    json:"external_id"`
	EmailVerified bool                       `bun:"email_verified,notnull" json:"email_verified"`
	TenantID      *string                    `bun:"tenant_id"              json:"tenant_id,omitempty"`
	TenantRole    *shared.CustomerTenantRole `bun:"tenant_role"            json:"tenant_role,omitempty"`
	PlanTier      *shared.CustomerPlanTier   `bun:"plan_tier"              json:"plan_tier,omitempty"`
	MFAEnabled    bool                       `bun:"mfa_enabled,notnull"    json:"mfa_enabled"`
	Description   *string                    `bun:"description"            json:"description,omitempty"`
	CreatedAt     time.Time                  `bun:"created_at,notnull"     json:"created_at"`
	UpdatedAt     time.Time                  `bun:"updated_at,notnull"     json:"updated_at"`
}

// CustomerAttribute is a single (key, value) tag attached to a
// Customer. (customer_id, key) is unique.
type CustomerAttribute struct {
	bun.BaseModel `bun:"table:customer_attributes,alias:ca"`

	ID         uuid.UUID `bun:"id,pk,type:uuid"     json:"id"`
	CustomerID uuid.UUID `bun:"customer_id,notnull" json:"customer_id"`
	Key        string    `bun:"key,notnull"         json:"key"`
	Value      string    `bun:"value,notnull"       json:"value"`
	CreatedAt  time.Time `bun:"created_at,notnull"  json:"created_at"`
}
