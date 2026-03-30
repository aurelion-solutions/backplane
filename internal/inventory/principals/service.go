// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package principals

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

// Service is the Principal use case layer.
type Service struct {
	repo    Repository
	sink    EventSink
	sources BodySources
	idGen   func() uuid.UUID
	now     func() time.Time
}

// Deps bundles cross-slice dependencies.
type Deps struct {
	Repo    Repository
	Sink    EventSink
	Sources BodySources
	IDGen   func() uuid.UUID
	Now     func() time.Time
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
		repo:    d.Repo,
		sink:    d.Sink,
		sources: d.Sources,
		idGen:   d.IDGen,
		now:     d.Now,
	}
}

// Create persists a new Principal. When status is omitted on the
// payload, derives it from current body state.
func (s *Service) Create(ctx context.Context, in CreatePayload) (*Principal, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	bodyID := bodyIDFromPayload(in)
	if err := s.checkBodyExists(ctx, in.Kind, bodyID); err != nil {
		return nil, err
	}
	status := ""
	if in.Status != nil {
		status = *in.Status
	} else {
		derived, err := DeriveStatus(ctx, s.sources, in.Kind, bodyID)
		if err != nil {
			return nil, err
		}
		status = derived
	}
	if !shared.StatusForKind(in.Kind, status) {
		return nil, fmt.Errorf("principals: derived status %q not in vocabulary for kind=%q", status, in.Kind)
	}
	now := s.now()
	row := &Principal{
		ID:                    s.idGen(),
		ExternalID:            strings.TrimSpace(in.ExternalID),
		Kind:                  in.Kind,
		PrincipalEmploymentID: in.PrincipalEmploymentID,
		PrincipalWorkloadID:   in.PrincipalWorkloadID,
		PrincipalCustomerID:   in.PrincipalCustomerID,
		Status:                status,
		IsLocked:              false,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := s.repo.Insert(ctx, row); err != nil {
		return nil, translateInsertError(err)
	}
	if err := s.emit(ctx, shared.EventPrincipalCreated, row.ID, map[string]any{
		"principal_id": row.ID.String(),
		"external_id":  row.ExternalID,
		"kind":         string(row.Kind),
		"status":       row.Status,
		"body_id":      bodyID.String(),
	}); err != nil {
		return nil, err
	}
	return row, nil
}

// Get returns one Principal.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Principal, error) {
	return s.repo.GetByID(ctx, id)
}

// GetByBody returns the principal row tied to the given body.
func (s *Service) GetByBody(ctx context.Context, kind shared.PrincipalKind, bodyID uuid.UUID) (*Principal, error) {
	return s.repo.GetByBody(ctx, kind, bodyID)
}

// List returns a paginated slice + total row count.
func (s *Service) List(ctx context.Context, limit, offset int) ([]*Principal, int, error) {
	return s.repo.List(ctx, limit, offset)
}

// ListAttributes returns every (key, value) tag attached to a Principal.
func (s *Service) ListAttributes(ctx context.Context, principalID uuid.UUID) ([]*PrincipalAttribute, error) {
	if _, err := s.repo.GetByID(ctx, principalID); err != nil {
		return nil, err
	}
	return s.repo.ListAttributes(ctx, principalID)
}

// Lock flips is_locked → true on the principal and emits
// inventory.principal.locked. Idempotent: a no-op (no event) when the
// principal is already locked.
func (s *Service) Lock(ctx context.Context, id uuid.UUID, in LockPayload) (*Principal, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.IsLocked {
		return p, nil
	}
	now := s.now()
	if err := s.repo.UpdateLock(ctx, p.ID, true, now); err != nil {
		return nil, err
	}
	p.IsLocked = true
	p.UpdatedAt = now
	payload := map[string]any{
		"principal_id": p.ID.String(),
		"kind":         string(p.Kind),
	}
	if in.Reason != nil {
		payload["reason"] = *in.Reason
	}
	if err := s.emit(ctx, shared.EventPrincipalLocked, p.ID, payload); err != nil {
		return nil, err
	}
	return p, nil
}

// Unlock flips is_locked → false on the principal and emits
// inventory.principal.unlocked. Idempotent on already-unlocked rows.
func (s *Service) Unlock(ctx context.Context, id uuid.UUID) (*Principal, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !p.IsLocked {
		return p, nil
	}
	now := s.now()
	if err := s.repo.UpdateLock(ctx, p.ID, false, now); err != nil {
		return nil, err
	}
	p.IsLocked = false
	p.UpdatedAt = now
	if err := s.emit(ctx, shared.EventPrincipalUnlocked, p.ID, map[string]any{
		"principal_id": p.ID.String(),
		"kind":         string(p.Kind),
	}); err != nil {
		return nil, err
	}
	return p, nil
}

// RecomputeForBody recomputes the derived status for the principal
// tied to (kind, bodyID) and persists the new value when it differs.
// Idempotent. Implements the cross-slice StatusRecomputer interface
// used by employments, workloads, and customers.
//
// If no principal exists for the body, returns ErrPrincipalMissingForBody.
func (s *Service) RecomputeForBody(ctx context.Context, kind shared.PrincipalKind, bodyID uuid.UUID) error {
	if !kind.Valid() {
		return ErrInvalidKind
	}
	p, err := s.repo.GetByBody(ctx, kind, bodyID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrPrincipalMissingForBody
		}
		return err
	}
	derived, err := DeriveStatus(ctx, s.sources, kind, bodyID)
	if err != nil {
		return err
	}
	if derived == p.Status {
		return nil
	}
	now := s.now()
	if err := s.repo.UpdateStatus(ctx, p.ID, derived, now); err != nil {
		return err
	}
	return s.emit(ctx, shared.EventPrincipalStatusRecomputed, p.ID, map[string]any{
		"principal_id": p.ID.String(),
		"kind":         string(kind),
		"old_status":   p.Status,
		"new_status":   derived,
	})
}

func (s *Service) checkBodyExists(ctx context.Context, kind shared.PrincipalKind, id uuid.UUID) error {
	switch kind {
	case shared.PrincipalKindEmployment:
		if s.sources.Employments == nil {
			return nil
		}
		_, ok, err := s.sources.Employments.EmploymentCode(ctx, id)
		if err != nil {
			return err
		}
		if !ok {
			return ErrBodyNotFound
		}
	case shared.PrincipalKindWorkload:
		if s.sources.Workloads == nil {
			return nil
		}
		exists, err := s.sources.Workloads.WorkloadExists(ctx, id)
		if err != nil {
			return err
		}
		if !exists {
			return ErrBodyNotFound
		}
	case shared.PrincipalKindCustomer:
		if s.sources.Customers == nil {
			return nil
		}
		v, err := s.sources.Customers.CustomerState(ctx, id)
		if err != nil {
			return err
		}
		if !v.Exists {
			return ErrBodyNotFound
		}
	}
	return nil
}

func bodyIDFromPayload(in CreatePayload) uuid.UUID {
	switch in.Kind {
	case shared.PrincipalKindEmployment:
		return *in.PrincipalEmploymentID
	case shared.PrincipalKindWorkload:
		return *in.PrincipalWorkloadID
	case shared.PrincipalKindCustomer:
		return *in.PrincipalCustomerID
	}
	return uuid.Nil
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
		ActorID:       shared.EventActorComponentPrincipals,
	}
	if target != uuid.Nil {
		in.TargetKind = events.ParticipantSystem
		in.TargetID = target.String()
	}
	env, err := events.NewEnvelope(in)
	if err != nil {
		return fmt.Errorf("principals: build event: %w", err)
	}
	if err := s.sink.Emit(ctx, env); err != nil {
		return fmt.Errorf("principals: emit event: %w", err)
	}
	return nil
}

func translateInsertError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr pgdriver.Error
	if errors.As(err, &pgErr) {
		switch pgErr.Field('C') {
		case "23505":
			msg := pgErr.Field('M')
			switch {
			case strings.Contains(msg, "uq_principals_kind_external_id"):
				return ErrDuplicate
			case strings.Contains(msg, "ux_principals_principal_"):
				return ErrBodyAlreadyBound
			}
		}
	}
	return err
}
