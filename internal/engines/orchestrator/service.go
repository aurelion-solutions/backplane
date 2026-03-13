// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package orchestrator

import (
	"context"

	"github.com/google/uuid"
)

// Service is the orchestrator's primary inbound API. HTTP routes and
// MQ matchers call into it. Persistence and dispatch are injected as
// ports so tests can swap them out.
type Service struct {
	repo       Repository
	loader     Loader
	dispatcher Dispatcher
}

// NewService composes the engine with its dependencies.
func NewService(repo Repository, loader Loader, dispatcher Dispatcher) *Service {
	return &Service{repo: repo, loader: loader, dispatcher: dispatcher}
}

// StartRun creates a new Run for the named Pipeline and dispatches the
// first wave of Steps. Skeleton: returns ErrNotImplemented until the
// real lifecycle is wired.
func (s *Service) StartRun(_ context.Context, _ string) (Run, error) {
	return Run{}, ErrNotImplemented
}

// GetRun fetches a Run by id. Skeleton.
func (s *Service) GetRun(_ context.Context, _ uuid.UUID) (Run, error) {
	return Run{}, ErrNotImplemented
}

// CancelRun marks a Run as cancelled and prevents further Steps from
// being dispatched. Skeleton.
func (s *Service) CancelRun(_ context.Context, _ uuid.UUID) error {
	return ErrNotImplemented
}

// ReportStepResult is the inbound callback from an executor that has
// finished a Step. Skeleton.
func (s *Service) ReportStepResult(_ context.Context, _ RunStep) error {
	return ErrNotImplemented
}
