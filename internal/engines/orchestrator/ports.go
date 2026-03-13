// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package orchestrator

import (
	"context"

	"github.com/google/uuid"
)

// Repository persists Pipelines, Runs and RunSteps. Backend-agnostic;
// the real implementation will sit on bun + Postgres.
type Repository interface {
	GetPipeline(ctx context.Context, name string) (Pipeline, error)
	CreateRun(ctx context.Context, r Run) error
	GetRun(ctx context.Context, id uuid.UUID) (Run, error)
	UpdateRunStatus(ctx context.Context, id uuid.UUID, status RunStatus, finishedAt, errMsg string) error

	CreateRunStep(ctx context.Context, s RunStep) error
	UpdateRunStep(ctx context.Context, s RunStep) error
	ListRunSteps(ctx context.Context, runID uuid.UUID) ([]RunStep, error)
}

// Loader reads Pipeline definitions out of a cartridge source (YAML
// today; OCI / git tomorrow).
type Loader interface {
	Load(ctx context.Context, cartridgeID string) ([]Pipeline, error)
}

// Dispatcher hands a RunStep off for execution. Today this means
// publishing on RabbitMQ; tomorrow it could be a Kafka or NATS queue.
type Dispatcher interface {
	Dispatch(ctx context.Context, s RunStep) error
}

// StepExecutor is the contract a single Step executor must satisfy.
// Lives in the worker binary and is selected by Step.Type.
type StepExecutor interface {
	// Type matches Step.Type at registration time.
	Type() string
	// Run performs the step. The returned map becomes RunStep.Output.
	Run(ctx context.Context, params map[string]any) (map[string]any, error)
}
