// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package inventory_discover

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/correlation"
	"github.com/aurelion-solutions/backplane/internal/core/events"
	"github.com/google/uuid"
)

// CommandDispatch is the port to the connector RPC channel. It must
// fire-and-forget a "go discover" command — the connector's reply
// comes later as MQ events, not as a synchronous RPC reply.
type CommandDispatch interface {
	Dispatch(ctx context.Context, cmd Command) error
}

// Command is the payload the engine asks CommandDispatch to send.
type Command struct {
	InstanceID    string
	Operation     string
	DatasetType   string
	CorrelationID string
	Payload       map[string]any
}

// EventSink mirrors core/events.Sink.
type EventSink interface {
	Emit(ctx context.Context, env events.Envelope) error
}

// Service is the use-case layer for orchestrating a pull.
type Service struct {
	repo     Repository
	dispatch CommandDispatch
	sink     EventSink
	idGen    func() uuid.UUID
	now      func() time.Time
}

// Deps bundles construction-time dependencies.
type Deps struct {
	Repo     Repository
	Dispatch CommandDispatch
	Sink     EventSink
	IDGen    func() uuid.UUID
	Now      func() time.Time
}

// NewService wires the Service.
func NewService(d Deps) *Service {
	if d.IDGen == nil {
		d.IDGen = uuid.New
	}
	if d.Now == nil {
		d.Now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{
		repo:     d.Repo,
		dispatch: d.Dispatch,
		sink:     d.Sink,
		idGen:    d.IDGen,
		now:      d.Now,
	}
}

// Fetch dispatches a discover command and records a DiscoverRun in
// the "dispatched" state. The connector's progress arrives later as
// MQ events; the subscriber updates the run row accordingly.
func (s *Service) Fetch(ctx context.Context, in FetchPayload) (*DiscoverRun, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}

	runID := s.idGen()
	// Use the run ID's string form as the correlation_id so every
	// connector-published record can be traced back to this run by
	// correlation_id alone.
	correlationID := runID.String()
	ctx = correlation.WithID(ctx, correlationID)

	run := &DiscoverRun{
		ID:                  runID,
		ConnectorInstanceID: strings.TrimSpace(in.ConnectorInstanceID),
		Operation:           strings.TrimSpace(in.Operation),
		DatasetType:         strings.TrimSpace(in.DatasetType),
		CorrelationID:       correlationID,
		Status:              StatusDispatched,
		StartedAt:           s.now(),
	}
	if err := s.repo.Insert(ctx, run); err != nil {
		return nil, fmt.Errorf("inventory_discover: insert run: %w", err)
	}

	if err := s.dispatch.Dispatch(ctx, Command{
		InstanceID:    run.ConnectorInstanceID,
		Operation:     run.Operation,
		DatasetType:   run.DatasetType,
		CorrelationID: correlationID,
		Payload:       in.Payload,
	}); err != nil {
		return s.markFailed(ctx, run, fmt.Errorf("%w: %v", ErrDispatch, err))
	}

	if err := s.emit(ctx, EventRunDispatched, run, nil); err != nil {
		return nil, err
	}
	return run, nil
}

// HandleConnectorEvent applies a connector lifecycle event to the
// matching DiscoverRun. eventType must be one of EventConnector*.
// runID resolution is by correlation_id — connectors stamp the same
// id their dispatch carried.
func (s *Service) HandleConnectorEvent(ctx context.Context, correlationID string, eventType string, payloadErr string) error {
	run, err := s.repo.GetByCorrelationID(ctx, correlationID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// Unknown correlation_id — most likely a stale message,
			// or a connector event from before our deployment. Ack
			// silently.
			return nil
		}
		return err
	}
	switch eventType {
	case EventConnectorStarted:
		if run.Status != StatusDispatched {
			return nil
		}
		run.Status = StatusRunning
		if err := s.repo.Update(ctx, run); err != nil {
			return fmt.Errorf("inventory_discover: mark running: %w", err)
		}
		return nil
	case EventConnectorCompleted:
		completedAt := s.now()
		run.Status = StatusCompleted
		run.CompletedAt = &completedAt
		if err := s.repo.Update(ctx, run); err != nil {
			return fmt.Errorf("inventory_discover: mark completed: %w", err)
		}
		return s.emit(ctx, EventRunCompleted, run, nil)
	case EventConnectorFailed:
		completedAt := s.now()
		msg := payloadErr
		run.Status = StatusFailed
		run.Error = &msg
		run.CompletedAt = &completedAt
		if err := s.repo.Update(ctx, run); err != nil {
			return fmt.Errorf("inventory_discover: mark failed: %w", err)
		}
		return s.emit(ctx, EventRunFailed, run, map[string]any{"error": msg})
	}
	return nil
}

// Get returns one run.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*DiscoverRun, error) {
	return s.repo.GetByID(ctx, id)
}

// List returns paginated runs + total.
func (s *Service) List(ctx context.Context, limit, offset int) ([]*DiscoverRun, int, error) {
	return s.repo.List(ctx, limit, offset)
}

func (s *Service) markFailed(ctx context.Context, run *DiscoverRun, cause error) (*DiscoverRun, error) {
	completedAt := s.now()
	msg := cause.Error()
	run.Status = StatusFailed
	run.Error = &msg
	run.CompletedAt = &completedAt
	if err := s.repo.Update(ctx, run); err != nil {
		return nil, fmt.Errorf("inventory_discover: mark failed: %w (cause: %v)", err, cause)
	}
	if err := s.emit(ctx, EventRunFailed, run, map[string]any{"error": msg}); err != nil {
		return nil, err
	}
	return run, cause
}

func (s *Service) emit(ctx context.Context, eventType string, run *DiscoverRun, extra map[string]any) error {
	if s.sink == nil {
		return nil
	}
	payload := map[string]any{
		"run_id":                run.ID.String(),
		"connector_instance_id": run.ConnectorInstanceID,
		"operation":             run.Operation,
		"dataset_type":          run.DatasetType,
		"status":                string(run.Status),
	}
	for k, v := range extra {
		payload[k] = v
	}
	env, err := events.NewEnvelope(events.EnvelopeInput{
		EventType:     eventType,
		CorrelationID: run.CorrelationID,
		Payload:       payload,
		ActorKind:     events.ParticipantComponent,
		ActorID:       EventActorComponent,
		TargetKind:    events.ParticipantCapability,
		TargetID:      run.ID.String(),
	})
	if err != nil {
		return fmt.Errorf("inventory_discover: build event: %w", err)
	}
	if err := s.sink.Emit(ctx, env); err != nil {
		return fmt.Errorf("inventory_discover: emit event: %w", err)
	}
	return nil
}
