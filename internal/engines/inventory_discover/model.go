// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package inventory_discover

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// RunStatus is the lifecycle of one discover run.
type RunStatus string

const (
	StatusDispatched RunStatus = "dispatched" // command published, not yet acked
	StatusRunning    RunStatus = "running"    // connector confirmed and is streaming
	StatusCompleted  RunStatus = "completed"  // connector signalled finished
	StatusFailed     RunStatus = "failed"     // connector signalled an error
	StatusTimedOut   RunStatus = "timed_out"  // sweeper marked it abandoned
)

// DiscoverRun is the audit row for one Fetch call. ReceivedCount and
// WrittenCount are aggregated from inventory_ingest.batch_received
// events that carry this run's correlation_id; they are eventually
// consistent.
type DiscoverRun struct {
	bun.BaseModel `bun:"table:inventory_discover_runs,alias:dr"`

	ID                  uuid.UUID  `bun:"id,pk,type:uuid"               json:"id"`
	ConnectorInstanceID string     `bun:"connector_instance_id,notnull" json:"connector_instance_id"`
	Operation           string     `bun:"operation,notnull"             json:"operation"`
	DatasetType         string     `bun:"dataset_type,notnull"          json:"dataset_type"`
	CorrelationID       string     `bun:"correlation_id,notnull"        json:"correlation_id"`
	Status              RunStatus  `bun:"status,notnull"                json:"status"`
	Error               *string    `bun:"error"                         json:"error,omitempty"`
	ReceivedCount       int        `bun:"received_count,notnull"        json:"received_count"`
	WrittenCount        int        `bun:"written_count,notnull"         json:"written_count"`
	StartedAt           time.Time  `bun:"started_at,notnull"            json:"started_at"`
	CompletedAt         *time.Time `bun:"completed_at"                  json:"completed_at,omitempty"`
}
