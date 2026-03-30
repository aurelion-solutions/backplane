// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package workloads owns the Workload entity — a non-human identity
// kind covering service accounts, machine identities, and other
// workload bodies. Mirrors the kernel NHI semantics for kind =
// "workload", as a first-class slice rather than a sub-kind of a
// generic NHI table. Future NHI kinds (api_keys, bots, certificates)
// get their own slice when they ship.
//
// There is intentionally NO is_locked column here. Access blocking
// for any identity (employment, workload, customer) lives on the
// Principal layer — that is the single point where access is granted
// or revoked. Workload carries owner + lifecycle facts only.
package workloads

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Workload is a persistent non-human identity row.
type Workload struct {
	bun.BaseModel `bun:"table:workloads,alias:w"`

	ID                uuid.UUID  `bun:"id,pk,type:uuid"        json:"id"`
	ExternalID        string     `bun:"external_id,notnull"    json:"external_id"`
	Name              string     `bun:"name,notnull"           json:"name"`
	Description       *string    `bun:"description"            json:"description,omitempty"`
	OwnerEmploymentID *uuid.UUID `bun:"owner_employment_id"    json:"owner_employment_id,omitempty"`
	ApplicationID     *uuid.UUID `bun:"application_id"         json:"application_id,omitempty"`
	CreatedAt         time.Time  `bun:"created_at,notnull"     json:"created_at"`
	UpdatedAt         time.Time  `bun:"updated_at,notnull"     json:"updated_at"`
}

// WorkloadAttribute is a single (key, value) tag attached to a Workload.
type WorkloadAttribute struct {
	bun.BaseModel `bun:"table:workload_attributes,alias:wa"`

	ID         uuid.UUID `bun:"id,pk,type:uuid"     json:"id"`
	WorkloadID uuid.UUID `bun:"workload_id,notnull" json:"workload_id"`
	Key        string    `bun:"key,notnull"         json:"key"`
	Value      string    `bun:"value,notnull"       json:"value"`
}
