// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package orchestrator

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrNotImplemented is returned by skeleton methods until the real
// implementation lands.
var ErrNotImplemented = errors.New("orchestrator: not implemented")

// ErrNotFound is returned when a Pipeline / Run / Step is not in storage.
var ErrNotFound = errors.New("orchestrator: not found")

// RunStatus describes the lifecycle of a Run.
type RunStatus string

const (
	RunPending   RunStatus = "pending"
	RunRunning   RunStatus = "running"
	RunSucceeded RunStatus = "succeeded"
	RunFailed    RunStatus = "failed"
	RunCancelled RunStatus = "cancelled"
)

// StepStatus describes the lifecycle of a RunStep.
type StepStatus string

const (
	StepPending   StepStatus = "pending"
	StepClaimed   StepStatus = "claimed"
	StepRunning   StepStatus = "running"
	StepSucceeded StepStatus = "succeeded"
	StepFailed    StepStatus = "failed"
	StepSkipped   StepStatus = "skipped"
)

// Step is one node in a Pipeline definition. Loaded from cartridge
// YAML, immutable at runtime.
type Step struct {
	ID     string         // unique within a Pipeline
	Type   string         // executor selector (e.g. "shell", "http", "sql")
	Params map[string]any // raw parameters; the executor interprets them
	After  []string       // step IDs that must finish before this one runs
}

// Pipeline is a directed acyclic graph of Steps, loaded from a cartridge.
type Pipeline struct {
	Name  string
	Steps []Step
}

// Run is a single execution attempt of a Pipeline.
type Run struct {
	ID         uuid.UUID
	Pipeline   string
	Status     RunStatus
	StartedAt  time.Time
	FinishedAt time.Time
	Error      string
}

// RunStep is the per-Step state inside a Run.
type RunStep struct {
	ID         uuid.UUID
	RunID      uuid.UUID
	StepID     string
	Status     StepStatus
	StartedAt  time.Time
	FinishedAt time.Time
	Output     map[string]any
	Error      string
}
