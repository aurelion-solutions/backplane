// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package orchestrator

import (
	"context"
	"log/slog"
	"time"
)

// Runner is the long-running loop that lives inside the executor
// binary. It claims dispatched RunSteps, looks up the matching
// StepExecutor by Type, runs it, and reports the result back via
// the Service.
type Runner struct {
	log       *slog.Logger
	service   *Service
	executors map[string]StepExecutor
}

// NewRunner composes a Runner with its dependencies. executors are
// keyed by their Type — duplicates overwrite silently for now.
func NewRunner(log *slog.Logger, service *Service, executors ...StepExecutor) *Runner {
	m := make(map[string]StepExecutor, len(executors))
	for _, e := range executors {
		m[e.Type()] = e
	}
	return &Runner{log: log, service: service, executors: m}
}

// Run blocks until ctx is cancelled. Skeleton: heartbeat loop, no
// actual step-claiming yet. Wire to the broker once Dispatcher and
// claim semantics are decided.
func (r *Runner) Run(ctx context.Context) error {
	r.log.Info("worker started",
		slog.Int("registered_executors", len(r.executors)),
	)
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			r.log.Info("worker stopping")
			return nil
		case <-t.C:
			r.log.Debug("worker heartbeat")
		}
	}
}
