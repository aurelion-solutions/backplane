// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package applications

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/correlation"
	"github.com/aurelion-solutions/backplane/internal/core/events"
	"github.com/google/uuid"
	"github.com/uptrace/bun/driver/pgdriver"
)

// EventSink mirrors core/events.Sink — kept locally so tests can
// substitute a recording stub without depending on package internals.
type EventSink interface {
	Emit(ctx context.Context, env events.Envelope) error
}

// Service holds the Application use cases. Constructed once in the
// composition root with a repository + event sink + clock.
type Service struct {
	repo   Repository
	events EventSink
	now    func() time.Time
}

// NewService wires the use case. now may be nil — the default is
// time.Now (production clock). Tests inject a fake.
func NewService(repo Repository, sink EventSink, now func() time.Time) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{repo: repo, events: sink, now: now}
}

// Create persists a fresh Application after validating the payload.
// Translates the (code) unique violation into ErrCodeAlreadyExists.
func (s *Service) Create(ctx context.Context, in CreatePayload) (*Application, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	now := s.now()
	app := &Application{
		ID:                    uuid.New(),
		Name:                  strings.TrimSpace(in.Name),
		Code:                  in.Code,
		Config:                normalizeConfig(in.Config),
		RequiredConnectorTags: normalizeTags(in.RequiredConnectorTags),
		IsActive:              true,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if in.IsActive != nil {
		app.IsActive = *in.IsActive
	}
	if err := s.repo.Insert(ctx, app); err != nil {
		return nil, translateInsertError(err, app.Code)
	}
	return app, nil
}

// Get returns one Application by id, or ErrNotFound.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Application, error) {
	return s.repo.GetByID(ctx, id)
}

// List returns every Application ordered by name ASC.
func (s *Service) List(ctx context.Context) ([]*Application, error) {
	return s.repo.List(ctx)
}

// Update applies a partial patch and writes the result back. Errors:
// ErrNotFound, ErrNoFields, ErrCodeAlreadyExists, plus validation.
func (s *Service) Update(ctx context.Context, id uuid.UUID, in PatchPayload) (*Application, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	app, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		app.Name = strings.TrimSpace(*in.Name)
	}
	if in.Code != nil {
		app.Code = *in.Code
	}
	if in.Config != nil {
		app.Config = normalizeConfig(in.Config)
	}
	if in.RequiredConnectorTags != nil {
		app.RequiredConnectorTags = normalizeTags(in.RequiredConnectorTags)
	}
	if in.IsActive != nil {
		app.IsActive = *in.IsActive
	}
	app.UpdatedAt = s.now()
	if err := s.repo.Update(ctx, app); err != nil {
		return nil, translateInsertError(err, app.Code)
	}
	return app, nil
}

// Decommission flips is_active to false and emits the
// inventory.application.decommissioned event. Idempotent: calling on an
// already-inactive Application is allowed (still emits the event).
func (s *Service) Decommission(ctx context.Context, id uuid.UUID) (*Application, error) {
	app, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	app.IsActive = false
	app.UpdatedAt = s.now()
	if err := s.repo.Update(ctx, app); err != nil {
		return nil, err
	}
	if s.events != nil {
		_, cid := correlation.Ensure(ctx)
		env, err := events.NewEnvelope(events.EnvelopeInput{
			EventType:     "inventory.application.decommissioned",
			CorrelationID: cid,
			Payload: map[string]any{
				"application_id": app.ID.String(),
				"code":           app.Code,
			},
			ActorKind:  events.ParticipantComponent,
			ActorID:    "integrations.applications",
			TargetKind: events.ParticipantApplication,
			TargetID:   app.ID.String(),
		})
		if err != nil {
			return nil, fmt.Errorf("applications: build event: %w", err)
		}
		if err := s.events.Emit(ctx, env); err != nil {
			return nil, fmt.Errorf("applications: emit event: %w", err)
		}
	}
	return app, nil
}

// Delete removes the Application or returns ErrNotFound.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

// translateInsertError maps a pgdriver unique-violation on
// uq_applications_code into ErrCodeAlreadyExists; everything else
// is returned verbatim so callers see the raw cause.
func translateInsertError(err error, code string) error {
	if err == nil {
		return nil
	}
	var pgErr pgdriver.Error
	if errors.As(err, &pgErr) {
		if pgErr.Field('C') == "23505" && strings.Contains(pgErr.Field('M'), "uq_applications_code") {
			return fmt.Errorf("%w: %s", ErrCodeAlreadyExists, code)
		}
	}
	return err
}

func normalizeConfig(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	return in
}

func normalizeTags(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}
