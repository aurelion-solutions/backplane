// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package employment_record_matches

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// EmploymentRecordMatch links a raw lake record to a resolved
// Employment for ONE employment period within that record.
//
// One source record (e.g. dataset_type=employee from HRIS) can carry
// several employment periods inline; each period gets its own match
// row, discriminated by PeriodStartDate.
//
// MatchedViaDeterminator distinguishes the authoritative match path
// (HRIS-style) from the upstream-attach path (AD/SAP-style).
type EmploymentRecordMatch struct {
	bun.BaseModel `bun:"table:employment_record_matches,alias:erm"`

	ID                     uuid.UUID `bun:"id,pk,type:uuid"                    json:"id"`
	EmploymentID           uuid.UUID `bun:"employment_id,notnull"              json:"employment_id"`
	Source                 string    `bun:"source,notnull"                     json:"source"`
	SourceRecordExternalID string    `bun:"source_record_external_id,notnull"  json:"source_record_external_id"`
	PeriodStartDate        time.Time `bun:"period_start_date,notnull,type:date" json:"period_start_date"`
	MatchedViaDeterminator bool      `bun:"matched_via_determinator,notnull"   json:"matched_via_determinator"`
	CreatedAt              time.Time `bun:"created_at,notnull"                 json:"created_at"`
	UpdatedAt              time.Time `bun:"updated_at,notnull"                 json:"updated_at"`
}
