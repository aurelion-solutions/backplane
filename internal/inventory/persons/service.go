// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package persons

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/correlation"
	"github.com/aurelion-solutions/backplane/internal/core/events"
	"github.com/aurelion-solutions/backplane/internal/inventory/shared"
	"github.com/google/uuid"
	"github.com/uptrace/bun/driver/pgdriver"
)

// EventSink mirrors core/events.Sink — kept locally so tests can
// substitute a recording stub.
type EventSink interface {
	Emit(ctx context.Context, env events.Envelope) error
}

// Service exposes the Person use cases.
type Service struct {
	repo  Repository
	sink  EventSink
	idGen func() uuid.UUID
}

// NewService wires a Service. idGen may be nil — defaults to uuid.New.
func NewService(repo Repository, sink EventSink, idGen func() uuid.UUID) *Service {
	if idGen == nil {
		idGen = uuid.New
	}
	return &Service{repo: repo, sink: sink, idGen: idGen}
}

// Create persists a fresh Person and emits inventory.person.created.
func (s *Service) Create(ctx context.Context, in CreatePayload) (*Person, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	p := &Person{
		ID:         s.idGen(),
		ExternalID: strings.TrimSpace(in.ExternalID),
		FullName:   strings.TrimSpace(in.FullName),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.repo.Insert(ctx, p); err != nil {
		return nil, translateInsertError(err, p.ExternalID)
	}
	if err := s.emit(ctx, shared.EventPersonCreated, p.ID, map[string]any{
		"person_id":   p.ID.String(),
		"external_id": p.ExternalID,
		"full_name":   p.FullName,
	}); err != nil {
		return nil, err
	}
	return p, nil
}

// Get returns one Person by id.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Person, error) {
	return s.repo.GetByID(ctx, id)
}

// List returns a paginated slice and the total row count.
func (s *Service) List(ctx context.Context, limit, offset int) ([]*Person, int, error) {
	return s.repo.List(ctx, limit, offset)
}

// ListAttributes returns every attribute of a person, sorted by key.
func (s *Service) ListAttributes(ctx context.Context, personID uuid.UUID) ([]*PersonAttribute, error) {
	if _, err := s.repo.GetByID(ctx, personID); err != nil {
		return nil, err
	}
	return s.repo.ListAttributes(ctx, personID)
}

// AddAttribute upserts a (person, key) attribute and emits
// inventory.person.attribute_added. Idempotent on key.
func (s *Service) AddAttribute(ctx context.Context, personID uuid.UUID, in AttributeCreatePayload) (*PersonAttribute, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.repo.GetByID(ctx, personID); err != nil {
		return nil, err
	}
	a := &PersonAttribute{
		ID:       s.idGen(),
		PersonID: personID,
		Key:      strings.TrimSpace(in.Key),
		Value:    in.Value,
	}
	if err := s.repo.UpsertAttribute(ctx, a); err != nil {
		return nil, err
	}
	if err := s.emit(ctx, shared.EventPersonAttributeAdded, personID, map[string]any{
		"person_id": personID.String(),
		"key":       a.Key,
	}); err != nil {
		return nil, err
	}
	// re-read to return the row that may have been updated, not the
	// one we tried to insert (id may differ on update).
	return s.repo.GetAttribute(ctx, personID, a.Key)
}

// RemoveAttribute deletes one (person, key) attribute, emitting
// inventory.person.attribute_removed on success.
func (s *Service) RemoveAttribute(ctx context.Context, personID uuid.UUID, key string) error {
	if _, err := s.repo.GetByID(ctx, personID); err != nil {
		return err
	}
	if err := s.repo.DeleteAttribute(ctx, personID, key); err != nil {
		return err
	}
	return s.emit(ctx, shared.EventPersonAttributeRemoved, personID, map[string]any{
		"person_id": personID.String(),
		"key":       key,
	})
}

// BulkUpsert reconciles a batch of person+attribute records and emits
// inventory.person.bulk_upserted with the row_count on success.
func (s *Service) BulkUpsert(ctx context.Context, in BulkPayload) (BulkResult, error) {
	if err := in.Validate(); err != nil {
		return BulkResult{}, err
	}
	n, err := s.repo.BulkUpsert(ctx, in.Items, s.idGen)
	if err != nil {
		return BulkResult{}, err
	}
	if err := s.emit(ctx, shared.EventPersonBulkUpserted, uuid.Nil, map[string]any{
		"row_count": n,
	}); err != nil {
		return BulkResult{}, err
	}
	return BulkResult{RowCount: n}, nil
}

func (s *Service) emit(ctx context.Context, eventType string, target uuid.UUID, payload map[string]any) error {
	if s.sink == nil {
		return nil
	}
	_, cid := correlation.Ensure(ctx)
	in := events.EnvelopeInput{
		EventType:     eventType,
		CorrelationID: cid,
		Payload:       payload,
		ActorKind:     events.ParticipantComponent,
		ActorID:       shared.EventActorComponentPersons,
	}
	if target != uuid.Nil {
		in.TargetKind = events.ParticipantUser
		in.TargetID = target.String()
	}
	env, err := events.NewEnvelope(in)
	if err != nil {
		return fmt.Errorf("persons: build event: %w", err)
	}
	if err := s.sink.Emit(ctx, env); err != nil {
		return fmt.Errorf("persons: emit event: %w", err)
	}
	return nil
}

// translateInsertError maps a Postgres unique violation on
// uq_persons_external_id into ErrExternalIDAlreadyExists. Other errors
// pass through verbatim.
func translateInsertError(err error, externalID string) error {
	if err == nil {
		return nil
	}
	var pgErr pgdriver.Error
	if errors.As(err, &pgErr) {
		if pgErr.Field('C') == "23505" && strings.Contains(pgErr.Field('M'), "uq_persons_external_id") {
			return fmt.Errorf("%w: %s", ErrExternalIDAlreadyExists, externalID)
		}
	}
	return err
}
