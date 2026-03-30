// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package org_units

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

// EventSink mirrors core/events.Sink.
type EventSink interface {
	Emit(ctx context.Context, env events.Envelope) error
}

// Service is the OrgUnit use case layer.
type Service struct {
	repo  Repository
	sink  EventSink
	idGen func() uuid.UUID
	now   func() time.Time
}

// NewService wires a Service.
func NewService(repo Repository, sink EventSink, idGen func() uuid.UUID, now func() time.Time) *Service {
	if idGen == nil {
		idGen = uuid.New
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{repo: repo, sink: sink, idGen: idGen, now: now}
}

// Create persists a new external OrgUnit (is_internal=false). Internal
// nodes are not creatable via service-layer API; they ship via
// migration / seed only.
//
// If parent_id is supplied, the parent must exist and must itself be
// external — trees do not cross the is_internal boundary.
func (s *Service) Create(ctx context.Context, in CreatePayload) (*OrgUnit, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	if in.ParentID != nil {
		parent, err := s.repo.GetByID(ctx, *in.ParentID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, ErrParentNotFound
			}
			return nil, err
		}
		if parent.IsInternal {
			return nil, ErrParentInternal
		}
	}
	now := s.now()
	u := &OrgUnit{
		ID:          s.idGen(),
		ExternalID:  strings.TrimSpace(in.ExternalID),
		Name:        strings.TrimSpace(in.Name),
		ParentID:    in.ParentID,
		Description: in.Description,
		IsInternal:  false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.repo.Insert(ctx, u); err != nil {
		return nil, translateInsertError(err, u.ExternalID)
	}
	if err := s.emit(ctx, shared.EventOrgUnitCreated, u.ID, map[string]any{
		"org_unit_id": u.ID.String(),
		"external_id": u.ExternalID,
		"parent_id":   uuidString(u.ParentID),
		"is_internal": u.IsInternal,
	}); err != nil {
		return nil, err
	}
	return u, nil
}

// Get returns one row.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*OrgUnit, error) {
	return s.repo.GetByID(ctx, id)
}

// List returns a paginated slice + total row count.
func (s *Service) List(ctx context.Context, limit, offset int) ([]*OrgUnit, int, error) {
	return s.repo.List(ctx, limit, offset)
}

// Update applies a name/description patch. Only external nodes may be
// patched; attempts on internal rows yield ErrCannotDeleteInternal
// (re-used for symmetry — "internal nodes are read-only").
func (s *Service) Update(ctx context.Context, id uuid.UUID, in PatchPayload) (*OrgUnit, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if u.IsInternal {
		return nil, ErrCannotDeleteInternal
	}
	changes := map[string]any{}
	if in.Name != nil {
		newName := strings.TrimSpace(*in.Name)
		if newName != u.Name {
			changes["name"] = map[string]any{"old": u.Name, "new": newName}
			u.Name = newName
		}
	}
	if in.Description != nil {
		old := stringDeref(u.Description)
		new := *in.Description
		if old != new {
			changes["description"] = map[string]any{"old": old, "new": new}
			cp := new
			u.Description = &cp
		}
	}
	if len(changes) == 0 {
		return u, nil
	}
	u.UpdatedAt = s.now()
	if err := s.repo.Update(ctx, u); err != nil {
		return nil, err
	}
	if err := s.emit(ctx, shared.EventOrgUnitUpdated, u.ID, map[string]any{
		"org_unit_id": u.ID.String(),
		"changes":     changes,
	}); err != nil {
		return nil, err
	}
	return u, nil
}

// Delete removes one row. Internal rows are read-only via the API.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if u.IsInternal {
		return ErrCannotDeleteInternal
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	return s.emit(ctx, shared.EventOrgUnitDeleted, id, map[string]any{
		"org_unit_id": id.String(),
		"external_id": u.ExternalID,
	})
}

// BulkUpsert reconciles a batch of external nodes. Emits
// inventory.org_unit.bulk_upserted with the row_count on success.
func (s *Service) BulkUpsert(ctx context.Context, in BulkPayload) (BulkResult, error) {
	if err := in.Validate(); err != nil {
		return BulkResult{}, err
	}
	n, err := s.repo.BulkUpsert(ctx, in.Items, s.idGen)
	if err != nil {
		return BulkResult{}, err
	}
	if err := s.emit(ctx, shared.EventOrgUnitBulkUpserted, uuid.Nil, map[string]any{
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
		ActorID:       shared.EventActorComponentOrgUnits,
	}
	if target != uuid.Nil {
		in.TargetKind = events.ParticipantSystem
		in.TargetID = target.String()
	}
	env, err := events.NewEnvelope(in)
	if err != nil {
		return fmt.Errorf("org_units: build event: %w", err)
	}
	if err := s.sink.Emit(ctx, env); err != nil {
		return fmt.Errorf("org_units: emit event: %w", err)
	}
	return nil
}

func translateInsertError(err error, externalID string) error {
	if err == nil {
		return nil
	}
	var pgErr pgdriver.Error
	if errors.As(err, &pgErr) {
		if pgErr.Field('C') == "23505" && strings.Contains(pgErr.Field('M'), "uq_org_units_external_id") {
			return fmt.Errorf("%w: %s", ErrExternalIDAlreadyExists, externalID)
		}
	}
	return err
}

func uuidString(u *uuid.UUID) any {
	if u == nil {
		return nil
	}
	return u.String()
}

func stringDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
