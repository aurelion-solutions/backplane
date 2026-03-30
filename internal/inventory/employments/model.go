// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package employments owns the Employment entity — a single working
// period of a Person at the customer. One Person may hold several
// concurrent or sequential Employments (e.g. full-time developer mask
// AND part-time QA mask at the same legal entity); each carries its
// own org-unit affiliation, job-title, manager, and access posture.
//
// The `code` column is tenant-defined free text — `active`,
// `probation`, `maternity_leave`, `notice_period`, `sabbatical`, etc.
// We do NOT enforce a closed vocabulary; every company labels their
// working states differently and we will not pretend otherwise.
//
// Subjects are tied to Employments (not Persons): when the developer
// mask is the active identity, the access policy looks at that
// Employment's subject; when the QA mask is the active one, a
// different Subject row applies. See the subjects slice.
package employments

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Employment is one working period of a Person.
//
// There is intentionally NO is_locked column here. Access blocking
// for any identity (employment, workload, customer) is the Principal
// layer's job — that is the single, kind-agnostic point where access
// is granted or revoked. Employment carries lifecycle (`code`,
// `start_date`, `end_date`); Principal carries the access posture.
type Employment struct {
	bun.BaseModel `bun:"table:employments,alias:em"`

	ID          uuid.UUID  `bun:"id,pk,type:uuid"     json:"id"`
	PersonID    uuid.UUID  `bun:"person_id,notnull"   json:"person_id"`
	Code        string     `bun:"code,notnull"        json:"code"`
	StartDate   time.Time  `bun:"start_date,notnull"  json:"start_date"`
	EndDate     *time.Time `bun:"end_date"            json:"end_date,omitempty"`
	OrgUnitID   *uuid.UUID `bun:"org_unit_id"         json:"org_unit_id,omitempty"`
	Description *string    `bun:"description"         json:"description,omitempty"`
	CreatedAt   time.Time  `bun:"created_at,notnull"  json:"created_at"`
	UpdatedAt   time.Time  `bun:"updated_at,notnull"  json:"updated_at"`
}

// IsActiveAt reports whether the employment is active on the given
// date (start_date ≤ date < end_date, treating NULL end_date as "open").
func (e *Employment) IsActiveAt(date time.Time) bool {
	d := date.UTC().Truncate(24 * time.Hour)
	if d.Before(e.StartDate.UTC().Truncate(24 * time.Hour)) {
		return false
	}
	if e.EndDate == nil {
		return true
	}
	return d.Before(e.EndDate.UTC().Truncate(24 * time.Hour))
}

// EmploymentAttribute is a period-specific (key, value) tag — things
// like job_title, manager_external_id, department_label, headcount
// allocation. Stable per-person attributes (name, primary email, DOB)
// live on PersonAttribute instead.
type EmploymentAttribute struct {
	bun.BaseModel `bun:"table:employment_attributes,alias:ea"`

	ID           uuid.UUID `bun:"id,pk,type:uuid"     json:"id"`
	EmploymentID uuid.UUID `bun:"employment_id,notnull" json:"employment_id"`
	Key          string    `bun:"key,notnull"         json:"key"`
	Value        string    `bun:"value,notnull"       json:"value"`
}
