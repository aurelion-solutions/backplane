// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package employments

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/correlation"
	"github.com/aurelion-solutions/backplane/internal/core/events"
	"github.com/aurelion-solutions/backplane/internal/inventory/shared"
	"github.com/google/uuid"
)

// EventSink mirrors core/events.Sink.
type EventSink interface {
	Emit(ctx context.Context, env events.Envelope) error
}

// PersonChecker reports whether a person id exists.
type PersonChecker interface {
	PersonExists(ctx context.Context, id uuid.UUID) (bool, error)
}

// OrgUnitChecker reports whether an org_unit id exists.
type OrgUnitChecker interface {
	OrgUnitExists(ctx context.Context, id uuid.UUID) (bool, error)
}

// PrincipalStatusRecomputer asks the principals slice to refresh an
// employment principal's derived status when employment.code changes.
type PrincipalStatusRecomputer interface {
	RecomputeForBody(ctx context.Context, kind shared.PrincipalKind, principalID uuid.UUID) error
}

// Service is the Employment use case layer.
type Service struct {
	repo            Repository
	sink            EventSink
	persons         PersonChecker
	orgUnits        OrgUnitChecker
	recomputer      PrincipalStatusRecomputer
	personResolver  PersonResolver
	orgUnitResolver OrgUnitResolver
	idGen           func() uuid.UUID
	now             func() time.Time
}

// Deps bundles cross-slice dependencies.
type Deps struct {
	Repo            Repository
	Sink            EventSink
	Persons         PersonChecker
	OrgUnits        OrgUnitChecker
	Recomputer      PrincipalStatusRecomputer
	PersonResolver  PersonResolver
	OrgUnitResolver OrgUnitResolver
	IDGen           func() uuid.UUID
	Now             func() time.Time
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
		repo:            d.Repo,
		sink:            d.Sink,
		persons:         d.Persons,
		orgUnits:        d.OrgUnits,
		recomputer:      d.Recomputer,
		personResolver:  d.PersonResolver,
		orgUnitResolver: d.OrgUnitResolver,
		idGen:           d.IDGen,
		now:             d.Now,
	}
}

// Create persists a fresh Employment and emits inventory.employment.created.
func (s *Service) Create(ctx context.Context, in CreatePayload) (*Employment, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	if s.persons != nil {
		ok, err := s.persons.PersonExists(ctx, in.PersonID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrPersonNotFound
		}
	}
	if in.OrgUnitID != nil && s.orgUnits != nil {
		ok, err := s.orgUnits.OrgUnitExists(ctx, *in.OrgUnitID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrOrgUnitNotFound
		}
	}
	now := s.now()
	e := &Employment{
		ID:          s.idGen(),
		PersonID:    in.PersonID,
		Code:        strings.TrimSpace(in.Code),
		StartDate:   in.StartDate.UTC(),
		OrgUnitID:   in.OrgUnitID,
		Description: in.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if in.EndDate != nil {
		ed := in.EndDate.UTC()
		e.EndDate = &ed
	}
	if err := s.repo.Insert(ctx, e); err != nil {
		return nil, err
	}
	if err := s.emit(ctx, shared.EventEmploymentCreated, e.ID, map[string]any{
		"employment_id": e.ID.String(),
		"person_id":     e.PersonID.String(),
		"code":          e.Code,
		"start_date":    e.StartDate.Format("2006-01-02"),
		"org_unit_id":   uuidString(e.OrgUnitID),
	}); err != nil {
		return nil, err
	}
	return e, nil
}

// Get returns one Employment.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Employment, error) {
	return s.repo.GetByID(ctx, id)
}

// List returns a paginated slice + total row count.
func (s *Service) List(ctx context.Context, limit, offset int) ([]*Employment, int, error) {
	return s.repo.List(ctx, limit, offset)
}

// ListByPerson returns every employment of a person, in start-date order.
func (s *Service) ListByPerson(ctx context.Context, personID uuid.UUID) ([]*Employment, error) {
	return s.repo.ListByPerson(ctx, personID)
}

// ListActiveByPerson returns the person's employments active on the
// given instant (`at`).
func (s *Service) ListActiveByPerson(ctx context.Context, personID uuid.UUID, at time.Time) ([]*Employment, error) {
	return s.repo.ListActiveByPerson(ctx, personID, at)
}

// Update applies an aggregate patch and emits one updated event with
// a `changes` map carrying every modified field. If code changes,
// asks the principals slice to recompute.
func (s *Service) Update(ctx context.Context, id uuid.UUID, in PatchPayload) (*Employment, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	e, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	changes := map[string]any{}
	codeChanged := false

	if in.Code != nil {
		newVal := strings.TrimSpace(*in.Code)
		if newVal != e.Code {
			changes["code"] = map[string]any{"old": e.Code, "new": newVal}
			e.Code = newVal
			codeChanged = true
		}
	}
	if in.StartDate != nil {
		newVal := in.StartDate.UTC()
		if !newVal.Equal(e.StartDate) {
			changes["start_date"] = map[string]any{
				"old": e.StartDate.Format("2006-01-02"),
				"new": newVal.Format("2006-01-02"),
			}
			e.StartDate = newVal
		}
	}
	if in.EndDate != nil {
		newVal := in.EndDate.UTC()
		oldStr := ""
		if e.EndDate != nil {
			oldStr = e.EndDate.Format("2006-01-02")
		}
		newStr := newVal.Format("2006-01-02")
		if oldStr != newStr {
			changes["end_date"] = map[string]any{"old": oldStr, "new": newStr}
			cp := newVal
			e.EndDate = &cp
		}
	}
	if e.EndDate != nil && e.EndDate.Before(e.StartDate) {
		return nil, ErrInvalidDates
	}
	if in.OrgUnitID != nil {
		if s.orgUnits != nil {
			ok, err := s.orgUnits.OrgUnitExists(ctx, *in.OrgUnitID)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, ErrOrgUnitNotFound
			}
		}
		oldVal := uuidString(e.OrgUnitID)
		newVal := in.OrgUnitID.String()
		if oldVal != newVal {
			changes["org_unit_id"] = map[string]any{"old": oldVal, "new": newVal}
			cp := *in.OrgUnitID
			e.OrgUnitID = &cp
		}
	}
	if in.Description != nil {
		old := stringDeref(e.Description)
		new := *in.Description
		if old != new {
			changes["description"] = map[string]any{"old": old, "new": new}
			cp := new
			e.Description = &cp
		}
	}
	if len(in.Attributes) > 0 {
		attrChanges := map[string]any{}
		for _, k := range sortedKeys(in.Attributes) {
			v := in.Attributes[k]
			existing, err := s.repo.GetAttribute(ctx, e.ID, k)
			oldVal := ""
			if err == nil {
				oldVal = existing.Value
			} else if !errors.Is(err, ErrAttributeNotFound) {
				return nil, err
			}
			if oldVal == v {
				continue
			}
			attrChanges[k] = map[string]any{"old": oldVal, "new": v}
			a := &EmploymentAttribute{ID: s.idGen(), EmploymentID: e.ID, Key: k, Value: v}
			if err := s.repo.UpsertAttribute(ctx, a); err != nil {
				return nil, err
			}
		}
		if len(attrChanges) > 0 {
			changes["attributes"] = attrChanges
		}
	}

	if len(changes) == 0 {
		return e, nil
	}
	e.UpdatedAt = s.now()
	if err := s.repo.Update(ctx, e); err != nil {
		return nil, err
	}
	if err := s.emit(ctx, shared.EventEmploymentUpdated, e.ID, map[string]any{
		"employment_id": e.ID.String(),
		"changes":       changes,
	}); err != nil {
		return nil, err
	}
	if codeChanged {
		if err := s.recompute(ctx, e.ID); err != nil {
			return nil, err
		}
	}
	return e, nil
}

// End stamps end_date and emits inventory.employment.ended. Idempotent
// on already-ended rows? No — already-ended rows raise ErrAlreadyEnded
// so accidental "end again" doesn't silently shift the boundary.
func (s *Service) End(ctx context.Context, id uuid.UUID, in EndPayload) (*Employment, error) {
	e, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if e.EndDate != nil {
		return nil, ErrAlreadyEnded
	}
	endDate := in.EndDate.UTC()
	if endDate.Before(e.StartDate) {
		return nil, ErrInvalidDates
	}
	e.EndDate = &endDate
	e.UpdatedAt = s.now()
	if err := s.repo.Update(ctx, e); err != nil {
		return nil, err
	}
	if err := s.emit(ctx, shared.EventEmploymentEnded, e.ID, map[string]any{
		"employment_id": e.ID.String(),
		"person_id":     e.PersonID.String(),
		"end_date":      endDate.Format("2006-01-02"),
	}); err != nil {
		return nil, err
	}
	if err := s.recompute(ctx, e.ID); err != nil {
		return nil, err
	}
	return e, nil
}

// ListAttributes returns every attribute of an employment.
func (s *Service) ListAttributes(ctx context.Context, id uuid.UUID) ([]*EmploymentAttribute, error) {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.ListAttributes(ctx, id)
}

// AddAttribute upserts an attribute and emits attribute_added.
func (s *Service) AddAttribute(ctx context.Context, id uuid.UUID, in AttributeCreatePayload) (*EmploymentAttribute, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return nil, err
	}
	a := &EmploymentAttribute{
		ID: s.idGen(), EmploymentID: id,
		Key: strings.TrimSpace(in.Key), Value: in.Value,
	}
	if err := s.repo.UpsertAttribute(ctx, a); err != nil {
		return nil, err
	}
	if err := s.emit(ctx, shared.EventEmploymentAttributeAdded, id, map[string]any{
		"employment_id": id.String(),
		"key":           a.Key,
	}); err != nil {
		return nil, err
	}
	return s.repo.GetAttribute(ctx, id, a.Key)
}

// RemoveAttribute deletes an attribute and emits attribute_removed.
func (s *Service) RemoveAttribute(ctx context.Context, id uuid.UUID, key string) error {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return err
	}
	if err := s.repo.DeleteAttribute(ctx, id, key); err != nil {
		return err
	}
	return s.emit(ctx, shared.EventEmploymentAttributeRemoved, id, map[string]any{
		"employment_id": id.String(),
		"key":           key,
	})
}

// BulkUpsert reconciles a batch and emits bulk_upserted.
func (s *Service) BulkUpsert(ctx context.Context, in BulkPayload) (BulkResult, error) {
	if err := in.Validate(); err != nil {
		return BulkResult{}, err
	}
	n, err := s.repo.BulkUpsert(ctx, in.Items, s.personResolver, s.orgUnitResolver, s.idGen)
	if err != nil {
		return BulkResult{}, err
	}
	if err := s.emit(ctx, shared.EventEmploymentBulkUpserted, uuid.Nil, map[string]any{
		"row_count": n,
	}); err != nil {
		return BulkResult{}, err
	}
	return BulkResult{RowCount: n}, nil
}

func (s *Service) recompute(ctx context.Context, employmentID uuid.UUID) error {
	if s.recomputer == nil {
		return nil
	}
	return s.recomputer.RecomputeForBody(ctx, shared.PrincipalKindEmployment, employmentID)
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
		ActorID:       shared.EventActorComponentEmployments,
	}
	if target != uuid.Nil {
		in.TargetKind = events.ParticipantUser
		in.TargetID = target.String()
	}
	env, err := events.NewEnvelope(in)
	if err != nil {
		return fmt.Errorf("employments: build event: %w", err)
	}
	if err := s.sink.Emit(ctx, env); err != nil {
		return fmt.Errorf("employments: emit event: %w", err)
	}
	return nil
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

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
