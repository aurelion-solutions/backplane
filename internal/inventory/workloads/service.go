// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package workloads

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
	"github.com/uptrace/bun/driver/pgdriver"
)

// EventSink mirrors core/events.Sink.
type EventSink interface {
	Emit(ctx context.Context, env events.Envelope) error
}

// EmploymentChecker reports whether an employment_id exists. Wired by
// the composition root via an employments-slice adapter.
type EmploymentChecker interface {
	EmploymentExists(ctx context.Context, id uuid.UUID) (bool, error)
}

// ApplicationChecker reports whether an application_id exists.
type ApplicationChecker interface {
	ApplicationExists(ctx context.Context, id uuid.UUID) (bool, error)
}

// Service is the Workload use case layer. Workload no longer carries
// is_locked or an Expire flow — both moved up to Principal. The
// service stays focused on owner / application wiring and attributes.
type Service struct {
	repo        Repository
	sink        EventSink
	employments EmploymentChecker
	apps        ApplicationChecker
	idGen       func() uuid.UUID
}

// Deps bundles cross-slice dependencies.
type Deps struct {
	Repo        Repository
	Sink        EventSink
	Employments EmploymentChecker
	Apps        ApplicationChecker
	IDGen       func() uuid.UUID
}

// NewService wires the Service.
func NewService(d Deps) *Service {
	if d.IDGen == nil {
		d.IDGen = uuid.New
	}
	return &Service{
		repo:        d.Repo,
		sink:        d.Sink,
		employments: d.Employments,
		apps:        d.Apps,
		idGen:       d.IDGen,
	}
}

// Create persists a fresh Workload and emits inventory.workload.created.
func (s *Service) Create(ctx context.Context, in CreatePayload) (*Workload, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	if err := s.checkOwner(ctx, in.OwnerEmploymentID); err != nil {
		return nil, err
	}
	if err := s.checkApplication(ctx, in.ApplicationID); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	w := &Workload{
		ID:                s.idGen(),
		ExternalID:        strings.TrimSpace(in.ExternalID),
		Name:              strings.TrimSpace(in.Name),
		Description:       in.Description,
		OwnerEmploymentID: in.OwnerEmploymentID,
		ApplicationID:     in.ApplicationID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.repo.Insert(ctx, w); err != nil {
		return nil, translateInsertError(err, w.ExternalID)
	}
	if err := s.emit(ctx, shared.EventWorkloadCreated, w.ID, map[string]any{
		"workload_id":         w.ID.String(),
		"external_id":         w.ExternalID,
		"owner_employment_id": uuidString(w.OwnerEmploymentID),
		"application_id":      uuidString(w.ApplicationID),
	}); err != nil {
		return nil, err
	}
	return w, nil
}

// Get returns one Workload.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Workload, error) {
	return s.repo.GetByID(ctx, id)
}

// List returns a paginated slice + total row count.
func (s *Service) List(ctx context.Context, limit, offset int) ([]*Workload, int, error) {
	return s.repo.List(ctx, limit, offset)
}

// Update applies an aggregate patch and emits one updated event.
func (s *Service) Update(ctx context.Context, id uuid.UUID, in PatchPayload) (*Workload, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	w, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	changes := map[string]any{}

	if in.Name != nil {
		newVal := strings.TrimSpace(*in.Name)
		if newVal != w.Name {
			changes["name"] = map[string]any{"old": w.Name, "new": newVal}
			w.Name = newVal
		}
	}
	if in.Description != nil {
		old := stringDeref(w.Description)
		new := *in.Description
		if old != new {
			changes["description"] = map[string]any{"old": old, "new": new}
			cp := new
			w.Description = &cp
		}
	}
	if in.OwnerEmploymentID != nil {
		if err := s.checkOwner(ctx, in.OwnerEmploymentID); err != nil {
			return nil, err
		}
		oldVal := uuidString(w.OwnerEmploymentID)
		newVal := in.OwnerEmploymentID.String()
		if oldVal != newVal {
			changes["owner_employment_id"] = map[string]any{"old": oldVal, "new": newVal}
			id := *in.OwnerEmploymentID
			w.OwnerEmploymentID = &id
		}
	}
	if in.ApplicationID != nil {
		if err := s.checkApplication(ctx, in.ApplicationID); err != nil {
			return nil, err
		}
		oldVal := uuidString(w.ApplicationID)
		newVal := in.ApplicationID.String()
		if oldVal != newVal {
			changes["application_id"] = map[string]any{"old": oldVal, "new": newVal}
			id := *in.ApplicationID
			w.ApplicationID = &id
		}
	}
	if len(in.Attributes) > 0 {
		attrChanges := map[string]any{}
		for _, k := range sortedKeys(in.Attributes) {
			v := in.Attributes[k]
			existing, err := s.repo.GetAttribute(ctx, w.ID, k)
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
			a := &WorkloadAttribute{ID: s.idGen(), WorkloadID: w.ID, Key: k, Value: v}
			if err := s.repo.UpsertAttribute(ctx, a); err != nil {
				return nil, err
			}
		}
		if len(attrChanges) > 0 {
			changes["attributes"] = attrChanges
		}
	}

	if len(changes) == 0 {
		return w, nil
	}
	w.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, w); err != nil {
		return nil, err
	}
	if err := s.emit(ctx, shared.EventWorkloadUpdated, w.ID, map[string]any{
		"workload_id": w.ID.String(),
		"changes":     changes,
	}); err != nil {
		return nil, err
	}
	return w, nil
}

// ListAttributes returns every attribute of a workload.
func (s *Service) ListAttributes(ctx context.Context, id uuid.UUID) ([]*WorkloadAttribute, error) {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.ListAttributes(ctx, id)
}

// AddAttribute upserts an attribute and emits attribute_added.
func (s *Service) AddAttribute(ctx context.Context, id uuid.UUID, in AttributeCreatePayload) (*WorkloadAttribute, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return nil, err
	}
	a := &WorkloadAttribute{ID: s.idGen(), WorkloadID: id, Key: strings.TrimSpace(in.Key), Value: in.Value}
	if err := s.repo.UpsertAttribute(ctx, a); err != nil {
		return nil, err
	}
	if err := s.emit(ctx, shared.EventWorkloadAttributeAdded, id, map[string]any{
		"workload_id": id.String(),
		"key":         a.Key,
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
	return s.emit(ctx, shared.EventWorkloadAttributeRemoved, id, map[string]any{
		"workload_id": id.String(),
		"key":         key,
	})
}

// BulkUpsert reconciles a batch and emits bulk_upserted.
func (s *Service) BulkUpsert(ctx context.Context, in BulkPayload) (BulkResult, error) {
	if err := in.Validate(); err != nil {
		return BulkResult{}, err
	}
	n, err := s.repo.BulkUpsert(ctx, in.Items, s.idGen)
	if err != nil {
		return BulkResult{}, err
	}
	if err := s.emit(ctx, shared.EventWorkloadBulkUpserted, uuid.Nil, map[string]any{
		"row_count": n,
	}); err != nil {
		return BulkResult{}, err
	}
	return BulkResult{RowCount: n}, nil
}

func (s *Service) checkOwner(ctx context.Context, id *uuid.UUID) error {
	if id == nil || s.employments == nil {
		return nil
	}
	ok, err := s.employments.EmploymentExists(ctx, *id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrOwnerNotFound
	}
	return nil
}

func (s *Service) checkApplication(ctx context.Context, id *uuid.UUID) error {
	if id == nil || s.apps == nil {
		return nil
	}
	ok, err := s.apps.ApplicationExists(ctx, *id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrApplicationNotFound
	}
	return nil
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
		ActorID:       shared.EventActorComponentWorkloads,
	}
	if target != uuid.Nil {
		in.TargetKind = events.ParticipantConnector
		in.TargetID = target.String()
	}
	env, err := events.NewEnvelope(in)
	if err != nil {
		return fmt.Errorf("workloads: build event: %w", err)
	}
	if err := s.sink.Emit(ctx, env); err != nil {
		return fmt.Errorf("workloads: emit event: %w", err)
	}
	return nil
}

func translateInsertError(err error, externalID string) error {
	if err == nil {
		return nil
	}
	var pgErr pgdriver.Error
	if errors.As(err, &pgErr) {
		if pgErr.Field('C') == "23505" && strings.Contains(pgErr.Field('M'), "uq_workloads_external_id") {
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

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
