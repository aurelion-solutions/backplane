// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package customers

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

// PrincipalStatusRecomputer asks the principals slice to refresh a
// customer principal's derived status. Wired by the composition root.
type PrincipalStatusRecomputer interface {
	RecomputeForBody(ctx context.Context, kind shared.PrincipalKind, principalID uuid.UUID) error
}

// Service is the Customer use case layer.
type Service struct {
	repo       Repository
	sink       EventSink
	recomputer PrincipalStatusRecomputer
	idGen      func() uuid.UUID
	now        func() time.Time
}

// Deps bundles cross-slice dependencies.
type Deps struct {
	Repo       Repository
	Sink       EventSink
	Recomputer PrincipalStatusRecomputer
	IDGen      func() uuid.UUID
	Now        func() time.Time
}

// NewService wires the Customer Service.
func NewService(d Deps) *Service {
	if d.IDGen == nil {
		d.IDGen = uuid.New
	}
	if d.Now == nil {
		d.Now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{
		repo:       d.Repo,
		sink:       d.Sink,
		recomputer: d.Recomputer,
		idGen:      d.IDGen,
		now:        d.Now,
	}
}

// Create persists a fresh Customer and emits inventory.customer.created.
func (s *Service) Create(ctx context.Context, in CreatePayload) (*Customer, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	now := s.now()
	c := &Customer{
		ID:          s.idGen(),
		ExternalID:  strings.TrimSpace(in.ExternalID),
		TenantID:    in.TenantID,
		TenantRole:  in.TenantRole,
		PlanTier:    in.PlanTier,
		Description: in.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	c.EmailVerified = boolDeref(in.EmailVerified, false)
	c.MFAEnabled = boolDeref(in.MFAEnabled, true)
	if err := s.repo.Insert(ctx, c); err != nil {
		return nil, translateInsertError(err, c.ExternalID)
	}
	planTier := ""
	if c.PlanTier != nil {
		planTier = string(*c.PlanTier)
	}
	tenantID := ""
	if c.TenantID != nil {
		tenantID = *c.TenantID
	}
	if err := s.emit(ctx, shared.EventCustomerCreated, c.ID, map[string]any{
		"customer_id": c.ID.String(),
		"external_id": c.ExternalID,
		"tenant_id":   tenantID,
		"plan_tier":   planTier,
	}); err != nil {
		return nil, err
	}
	return c, nil
}

// Get returns one Customer.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Customer, error) {
	return s.repo.GetByID(ctx, id)
}

// List returns a paginated slice + total row count.
func (s *Service) List(ctx context.Context, limit, offset int) ([]*Customer, int, error) {
	return s.repo.List(ctx, limit, offset)
}

// Update applies the strict 4-field patch and emits one updated event
// listing the changed fields. Triggers subject status recompute when
// email_verified or is_locked transitions.
func (s *Service) Update(ctx context.Context, id uuid.UUID, in PatchPayload) (*Customer, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	changed := []string{}
	recompute := false

	if in.EmailVerified != nil && *in.EmailVerified != c.EmailVerified {
		c.EmailVerified = *in.EmailVerified
		changed = append(changed, "email_verified")
		recompute = true
	}
	if in.MFAEnabled != nil && *in.MFAEnabled != c.MFAEnabled {
		c.MFAEnabled = *in.MFAEnabled
		changed = append(changed, "mfa_enabled")
	}
	if in.PlanTier != nil {
		old := ""
		if c.PlanTier != nil {
			old = string(*c.PlanTier)
		}
		newVal := string(*in.PlanTier)
		if old != newVal {
			pt := *in.PlanTier
			c.PlanTier = &pt
			changed = append(changed, "plan_tier")
		}
	}

	if len(changed) == 0 {
		return c, nil
	}
	sort.Strings(changed)
	c.UpdatedAt = s.now()
	if err := s.repo.Update(ctx, c); err != nil {
		return nil, err
	}
	if err := s.emit(ctx, shared.EventCustomerUpdated, c.ID, map[string]any{
		"customer_id":    c.ID.String(),
		"changed_fields": changed,
	}); err != nil {
		return nil, err
	}
	if recompute {
		if err := s.recompute(ctx, c.ID); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// ListAttributes returns every attribute of a customer.
func (s *Service) ListAttributes(ctx context.Context, id uuid.UUID) ([]*CustomerAttribute, error) {
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.ListAttributes(ctx, id)
}

// AddAttribute upserts an attribute and emits attribute_added.
func (s *Service) AddAttribute(ctx context.Context, id uuid.UUID, in AttributeCreatePayload) (*CustomerAttribute, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.repo.GetByID(ctx, id); err != nil {
		return nil, err
	}
	a := &CustomerAttribute{
		ID:         s.idGen(),
		CustomerID: id,
		Key:        strings.TrimSpace(in.Key),
		Value:      in.Value,
		CreatedAt:  s.now(),
	}
	if err := s.repo.UpsertAttribute(ctx, a); err != nil {
		return nil, err
	}
	if err := s.emit(ctx, shared.EventCustomerAttributeAdded, id, map[string]any{
		"customer_id": id.String(),
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
	return s.emit(ctx, shared.EventCustomerAttributeRemoved, id, map[string]any{
		"customer_id": id.String(),
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
	if err := s.emit(ctx, shared.EventCustomerBulkUpserted, uuid.Nil, map[string]any{
		"row_count": n,
	}); err != nil {
		return BulkResult{}, err
	}
	return BulkResult{RowCount: n}, nil
}

func (s *Service) recompute(ctx context.Context, customerID uuid.UUID) error {
	if s.recomputer == nil {
		return nil
	}
	return s.recomputer.RecomputeForBody(ctx, shared.PrincipalKindCustomer, customerID)
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
		ActorID:       shared.EventActorComponentCustomers,
	}
	if target != uuid.Nil {
		in.TargetKind = events.ParticipantUser
		in.TargetID = target.String()
	}
	env, err := events.NewEnvelope(in)
	if err != nil {
		return fmt.Errorf("customers: build event: %w", err)
	}
	if err := s.sink.Emit(ctx, env); err != nil {
		return fmt.Errorf("customers: emit event: %w", err)
	}
	return nil
}

func translateInsertError(err error, externalID string) error {
	if err == nil {
		return nil
	}
	var pgErr pgdriver.Error
	if errors.As(err, &pgErr) {
		if pgErr.Field('C') == "23505" && strings.Contains(pgErr.Field('M'), "uq_customers_external_id") {
			return fmt.Errorf("%w: %s", ErrExternalIDAlreadyExists, externalID)
		}
	}
	return err
}
