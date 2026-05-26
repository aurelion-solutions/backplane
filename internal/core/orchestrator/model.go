// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package orchestrator

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// RunStatus enumerates the lifecycle of a PipelineRun row.
//
// 'aborted' is NOT a valid run status — it is reserved for the
// per-attempt step_run row that the reclaim transaction abandons.
type RunStatus string

const (
	RunPending       RunStatus = "pending"
	RunRunning       RunStatus = "running"
	RunAwaitingEvent RunStatus = "awaiting_event"
	RunCancelling    RunStatus = "cancelling"
	RunCompleted     RunStatus = "completed"
	RunFailed        RunStatus = "failed"
	RunFailedTimeout RunStatus = "failed_timeout"
	RunCancelled     RunStatus = "cancelled"
)

// IsTerminal reports whether the status is one the runner won't
// re-enter.
func (s RunStatus) IsTerminal() bool {
	switch s {
	case RunCompleted, RunFailed, RunFailedTimeout, RunCancelled:
		return true
	}
	return false
}

// StepStatus enumerates the lifecycle of one step attempt row.
type StepStatus string

const (
	StepPending       StepStatus = "pending"
	StepRunning       StepStatus = "running"
	StepAwaitingEvent StepStatus = "awaiting_event"
	StepCompleted     StepStatus = "completed"
	StepFailed        StepStatus = "failed"
	StepFailedTimeout StepStatus = "failed_timeout"
	StepAborted       StepStatus = "aborted"
	StepCancelled     StepStatus = "cancelled"
)

// WaiterStatus enumerates the lifecycle of an in-flight event waiter.
type WaiterStatus string

const (
	WaiterWaiting   WaiterStatus = "waiting"
	WaiterMatched   WaiterStatus = "matched"
	WaiterExpired   WaiterStatus = "expired"
	WaiterCancelled WaiterStatus = "cancelled"
)

// TriggerSource identifies what caused a pipeline run to be created.
//
// 'http' covers manual POST /pipelines/{name}/runs; schedule and mq
// triggers are declared in pipeline YAML. 'retry' is the source set on
// a row produced by POST /pipelines/runs/{id}/retry.
type TriggerSource string

const (
	TriggerHTTP     TriggerSource = "http"
	TriggerMQ       TriggerSource = "mq"
	TriggerSchedule TriggerSource = "schedule"
	TriggerRetry    TriggerSource = "retry"
)

// WorkerSlot is the registry row for one live runner slot. Each
// worker process upserts on startup, ticks last_heartbeat_at on a
// fixed cadence, and deletes on graceful shutdown.
type WorkerSlot struct {
	bun.BaseModel `bun:"table:worker_slots,alias:ws"`

	WorkerID        string    `bun:"worker_id,pk"                json:"worker_id"`
	Hostname        string    `bun:"hostname,notnull"            json:"hostname"`
	PID             int       `bun:"pid,notnull"                 json:"pid"`
	SlotIndex       int       `bun:"slot_index,notnull"          json:"slot_index"`
	StartedAt       time.Time `bun:"started_at,notnull"          json:"started_at"`
	LastHeartbeatAt time.Time `bun:"last_heartbeat_at,notnull"   json:"last_heartbeat_at"`
	Tags            []string  `bun:"tags,array"                  json:"tags"`
}

// WorkerSummary is the per-slot view returned by GET /api/v0/workers.
// Combines the registry row (WorkerSlot — always present for live
// slots) with the derived active-runs aggregate from pipeline_runs
// (zero for idle slots).
type WorkerSummary struct {
	WorkerID           string     `bun:"worker_id"             json:"worker_id"`
	Hostname           string     `bun:"hostname"              json:"hostname"`
	PID                int        `bun:"pid"                   json:"pid"`
	SlotIndex          int        `bun:"slot_index"            json:"slot_index"`
	StartedAt          time.Time  `bun:"started_at"            json:"started_at"`
	LastHeartbeatAt    time.Time  `bun:"last_heartbeat_at"     json:"last_heartbeat_at"`
	Tags               []string   `bun:"tags,array"            json:"tags"`
	ActiveRuns         int        `bun:"active_runs"           json:"active_runs"`
	EarliestRunStartAt *time.Time `bun:"earliest_run_start_at" json:"earliest_run_started_at,omitempty"`
	// CurrentRunID + CurrentPipeline are populated when the slot
	// currently holds at least one run (status='running' | 'cancelling').
	// `MIN(id)` is used to pick a stable representative when a slot
	// holds more than one — that never happens in the current runner
	// (one in-flight run per slot), but the join is defensive.
	CurrentRunID    *string `bun:"current_run_id"  json:"current_run_id,omitempty"`
	CurrentPipeline *string `bun:"current_pipeline" json:"current_pipeline_name,omitempty"`
}

// PipelineRun is one execution attempt of a named pipeline definition.
type PipelineRun struct {
	bun.BaseModel `bun:"table:pipeline_runs,alias:pr"`

	ID              uuid.UUID      `bun:"id,pk,type:uuid"        json:"id"`
	PipelineName    string         `bun:"pipeline_name,notnull"  json:"pipeline_name"`
	PipelineVersion int            `bun:"pipeline_version,notnull" json:"pipeline_version"`
	Args            map[string]any `bun:"args,type:jsonb,notnull"  json:"args"`
	ContentHash     string         `bun:"content_hash,notnull"   json:"content_hash"`
	Status          RunStatus      `bun:"status,notnull"         json:"status"`
	CurrentStep     *string        `bun:"current_step"           json:"current_step,omitempty"`
	RetryOfRunID    *uuid.UUID     `bun:"retry_of_run_id"        json:"retry_of_run_id,omitempty"`
	TriggerSource   TriggerSource  `bun:"trigger_source,notnull" json:"trigger_source"`
	StartedAt       *time.Time     `bun:"started_at"             json:"started_at,omitempty"`
	FinishedAt      *time.Time     `bun:"finished_at"            json:"finished_at,omitempty"`
	Error           *string        `bun:"error"                  json:"error,omitempty"`
	WorkerID        *string        `bun:"worker_id"              json:"worker_id,omitempty"`
	LastHeartbeatAt *time.Time     `bun:"last_heartbeat_at"      json:"last_heartbeat_at,omitempty"`
	CreatedAt       time.Time      `bun:"created_at,notnull"     json:"created_at"`
	UpdatedAt       time.Time      `bun:"updated_at,notnull"     json:"updated_at"`
}

// StepRun is one attempt of one named step within a PipelineRun.
//
// (pipeline_run_id, step_name, attempt) is UNIQUE — at most one
// in-flight attempt per (run, step). Reclaim inserts a new attempt and
// flips the previous one to 'aborted' in the same transaction.
type StepRun struct {
	bun.BaseModel `bun:"table:step_runs,alias:sr"`

	ID            uuid.UUID      `bun:"id,pk,type:uuid"           json:"id"`
	PipelineRunID uuid.UUID      `bun:"pipeline_run_id,notnull"   json:"pipeline_run_id"`
	StepName      string         `bun:"step_name,notnull"         json:"step_name"`
	Attempt       int            `bun:"attempt,notnull"           json:"attempt"`
	Status        StepStatus     `bun:"status,notnull"            json:"status"`
	Args          map[string]any `bun:"args,type:jsonb,notnull"   json:"args"`
	Result        map[string]any `bun:"result,type:jsonb"         json:"result,omitempty"`
	Error         *string        `bun:"error"                     json:"error,omitempty"`
	StartedAt     *time.Time     `bun:"started_at"                json:"started_at,omitempty"`
	FinishedAt    *time.Time     `bun:"finished_at"               json:"finished_at,omitempty"`
	CreatedAt     time.Time      `bun:"created_at,notnull"        json:"created_at"`
	UpdatedAt     time.Time      `bun:"updated_at,notnull"        json:"updated_at"`
}

// EventWaiter is one in-flight wait_for_event parking row. A
// step_run_id has at most one active waiter. The HITL resolver and the
// matcher both write 'matched' through Service.ResolveWaiter.
type EventWaiter struct {
	bun.BaseModel `bun:"table:pipeline_event_waiters,alias:ew"`

	ID        uuid.UUID      `bun:"id,pk,type:uuid"        json:"id"`
	StepRunID uuid.UUID      `bun:"step_run_id,notnull"    json:"step_run_id"`
	EventType string         `bun:"event_type,notnull"     json:"event_type"`
	Match     map[string]any `bun:"match,type:jsonb,notnull" json:"match"`
	ExpiresAt time.Time      `bun:"expires_at,notnull"     json:"expires_at"`
	Status    WaiterStatus   `bun:"status,notnull"         json:"status"`
	CreatedAt time.Time      `bun:"created_at,notnull"     json:"created_at"`
}
