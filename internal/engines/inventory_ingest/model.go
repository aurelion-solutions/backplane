// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package inventory_ingest

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// IngestBatch is the audit row for one Process call. The lake bytes
// live elsewhere; this row exists so an operator can answer "what
// did doer X bring through correlation Y, and how much of it
// changed".
//
// LakeRef is nullable: when every incoming record was unchanged we
// did not write a lake file at all, only a count was recorded.
type IngestBatch struct {
	bun.BaseModel `bun:"table:inventory_ingest_batches,alias:ib"`

	ID            uuid.UUID `bun:"id,pk,type:uuid"          json:"id"`
	Source        string    `bun:"source,notnull"           json:"source"`
	DatasetType   string    `bun:"dataset_type,notnull"     json:"dataset_type"`
	CorrelationID string    `bun:"correlation_id,notnull"   json:"correlation_id"`
	ReceivedCount int       `bun:"received_count,notnull"   json:"received_count"`
	WrittenCount  int       `bun:"written_count,notnull"    json:"written_count"`
	SkippedCount  int       `bun:"skipped_count,notnull"    json:"skipped_count"`
	NewCount      int       `bun:"new_count,notnull"        json:"new_count"`
	ChangedCount  int       `bun:"changed_count,notnull"    json:"changed_count"`
	LakeRef       *string   `bun:"lake_ref"                 json:"lake_ref,omitempty"`
	CompletedAt   time.Time `bun:"completed_at,notnull"     json:"completed_at"`
}
