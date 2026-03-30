// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package employee_records owns the EmployeeRecord entity — the
// external source-side row representing a person as a given application
// sees them. EmployeeRecords are matched to canonical Employees via
// EmployeeRecordMatch (1:1, owned by record), driven by per-application
// EmployeeProviderAttributeMapping rules. The Resolver glues these
// together — see resolver.go.
package employee_records //nolint:revive,stylecheck // package name matches the slice path

import (
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// EmployeeRecord is the source-side row from one application about one
// person. Unique per (application_id, external_id).
type EmployeeRecord struct {
	bun.BaseModel `bun:"table:employee_records,alias:er"`

	ID            uuid.UUID `bun:"id,pk,type:uuid"        json:"id"`
	ExternalID    string    `bun:"external_id,notnull"    json:"external_id"`
	ApplicationID uuid.UUID `bun:"application_id,notnull" json:"application_id"`
	Description   *string   `bun:"description"            json:"description,omitempty"`
}

// EmployeeRecordAttribute is one (key, value) tag on a record. The
// (employee_record_id, key) pair is unique.
type EmployeeRecordAttribute struct {
	bun.BaseModel `bun:"table:employee_record_attributes,alias:era"`

	ID               uuid.UUID `bun:"id,pk,type:uuid"           json:"id"`
	EmployeeRecordID uuid.UUID `bun:"employee_record_id,notnull" json:"employee_record_id"`
	Key              string    `bun:"key,notnull"               json:"key"`
	Value            string    `bun:"value,notnull"             json:"value"`
}

// EmployeeProviderAttributeMapping is a per-application rule that maps
// an EmployeeRecord attribute key to a Person attribute key. Two
// boolean toggles:
//
//   - is_determinator: this mapping drives resolver lookup. The
//     resolver reads the record value under employee_record_key, then
//     looks up the canonical Person by person_key.
//   - allow_upstream: this mapping is usable as a peer-linking edge.
//     When no determinator fires, the resolver follows allow_upstream
//     edges to find sibling records that resolve.
type EmployeeProviderAttributeMapping struct {
	bun.BaseModel `bun:"table:employee_provider_attribute_mappings,alias:epam"`

	ID                uuid.UUID `bun:"id,pk,type:uuid"             json:"id"`
	ApplicationID     uuid.UUID `bun:"application_id,notnull"      json:"application_id"`
	EmployeeRecordKey string    `bun:"employee_record_key,notnull" json:"employee_record_key"`
	PersonKey         string    `bun:"person_key,notnull"          json:"person_key"`
	IsDeterminator    bool      `bun:"is_determinator,notnull"     json:"is_determinator"`
	AllowUpstream     bool      `bun:"allow_upstream,notnull"      json:"allow_upstream"`
}

// EmployeeRecordMatch binds a record to the canonical Person + the
// specific Employment "mask" that the record represents. 1:1 with
// EmployeeRecord (enforced by unique key on employee_record_id);
// only the current match is stored.
type EmployeeRecordMatch struct {
	bun.BaseModel `bun:"table:employee_record_matches,alias:erm"`

	ID                     uuid.UUID `bun:"id,pk,type:uuid"                  json:"id"`
	EmployeeRecordID       uuid.UUID `bun:"employee_record_id,notnull"       json:"employee_record_id"`
	PersonID               uuid.UUID `bun:"person_id,notnull"                json:"person_id"`
	EmploymentID           uuid.UUID `bun:"employment_id,notnull"            json:"employment_id"`
	MatchedViaDeterminator bool      `bun:"matched_via_determinator,notnull" json:"matched_via_determinator"`
}
