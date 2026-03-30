// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package employee_records

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

// ApplicationChecker reports whether an application_id exists.
type ApplicationChecker interface {
	ApplicationExists(ctx context.Context, id uuid.UUID) (bool, error)
}

// PersonChecker reports whether a person_id exists.
type PersonChecker interface {
	PersonExists(ctx context.Context, id uuid.UUID) (bool, error)
}

// EmploymentChecker reports whether an employment_id exists and is
// owned by the given person.
type EmploymentChecker interface {
	EmploymentExistsForPerson(ctx context.Context, employmentID, personID uuid.UUID) (bool, error)
}

// Service is the EmployeeRecord use case layer.
type Service struct {
	repo        Repository
	sink        EventSink
	apps        ApplicationChecker
	persons     PersonChecker
	employments EmploymentChecker
	appsRes     ApplicationResolver
	resolver    *Resolver
	idGen       func() uuid.UUID
}

// Deps bundles cross-slice dependencies.
type Deps struct {
	Repo         Repository
	Sink         EventSink
	Apps         ApplicationChecker
	Persons      PersonChecker
	Employments  EmploymentChecker
	AppsResolver ApplicationResolver
	Resolver     *Resolver
	IDGen        func() uuid.UUID
}

// NewService wires the Service.
func NewService(d Deps) *Service {
	if d.IDGen == nil {
		d.IDGen = uuid.New
	}
	return &Service{
		repo:        d.Repo,
		sink:        d.Sink,
		apps:        d.Apps,
		persons:     d.Persons,
		employments: d.Employments,
		appsRes:     d.AppsResolver,
		resolver:    d.Resolver,
		idGen:       d.IDGen,
	}
}

// CreateRecord persists a new EmployeeRecord and emits
// inventory.employee_record.created.
func (s *Service) CreateRecord(ctx context.Context, in CreatePayload) (*EmployeeRecord, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	if s.apps != nil {
		ok, err := s.apps.ApplicationExists(ctx, in.ApplicationID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrApplicationNotFound
		}
	}
	row := &EmployeeRecord{
		ID:            s.idGen(),
		ExternalID:    strings.TrimSpace(in.ExternalID),
		ApplicationID: in.ApplicationID,
		Description:   in.Description,
	}
	if err := s.repo.InsertRecord(ctx, row); err != nil {
		return nil, translateRecordInsertError(err)
	}
	desc := ""
	if row.Description != nil {
		desc = *row.Description
	}
	if err := s.emit(ctx, shared.EventEmployeeRecordCreated, row.ID, map[string]any{
		"employee_record_id": row.ID.String(),
		"external_id":        row.ExternalID,
		"application_id":     row.ApplicationID.String(),
		"description":        desc,
	}); err != nil {
		return nil, err
	}
	return row, nil
}

// GetRecord returns one record.
func (s *Service) GetRecord(ctx context.Context, id uuid.UUID) (*EmployeeRecord, error) {
	return s.repo.GetRecordByID(ctx, id)
}

// ListRecords returns every record (no pagination — matches kernel).
func (s *Service) ListRecords(ctx context.Context) ([]*EmployeeRecord, error) {
	out, err := s.repo.ListRecords(ctx)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return []*EmployeeRecord{}, nil
	}
	return out, nil
}

// ListAttributes returns every attribute of a record.
func (s *Service) ListAttributes(ctx context.Context, id uuid.UUID) ([]*EmployeeRecordAttribute, error) {
	if _, err := s.repo.GetRecordByID(ctx, id); err != nil {
		return nil, err
	}
	return s.repo.ListRecordAttributes(ctx, id)
}

// AddAttribute upserts an attribute and emits attribute_added.
func (s *Service) AddAttribute(ctx context.Context, id uuid.UUID, in AttributeCreatePayload) (*EmployeeRecordAttribute, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.repo.GetRecordByID(ctx, id); err != nil {
		return nil, err
	}
	a := &EmployeeRecordAttribute{
		ID:               s.idGen(),
		EmployeeRecordID: id,
		Key:              strings.TrimSpace(in.Key),
		Value:            in.Value,
	}
	if err := s.repo.UpsertRecordAttribute(ctx, a); err != nil {
		return nil, err
	}
	if err := s.emit(ctx, shared.EventEmployeeRecordAttributeAdded, id, map[string]any{
		"employee_record_id": id.String(),
		"key":                a.Key,
	}); err != nil {
		return nil, err
	}
	return s.repo.GetRecordAttribute(ctx, id, a.Key)
}

// RemoveAttribute deletes an attribute and emits attribute_removed.
func (s *Service) RemoveAttribute(ctx context.Context, id uuid.UUID, key string) error {
	if _, err := s.repo.GetRecordByID(ctx, id); err != nil {
		return err
	}
	if err := s.repo.DeleteRecordAttribute(ctx, id, key); err != nil {
		return err
	}
	return s.emit(ctx, shared.EventEmployeeRecordAttributeRemoved, id, map[string]any{
		"employee_record_id": id.String(),
		"key":                key,
	})
}

// BulkUpsert reconciles a batch and emits bulk_upserted.
func (s *Service) BulkUpsert(ctx context.Context, in BulkPayload) (BulkResult, error) {
	if err := in.Validate(); err != nil {
		return BulkResult{}, err
	}
	n, err := s.repo.BulkUpsert(ctx, in.Items, s.appsRes, s.idGen)
	if err != nil {
		return BulkResult{}, err
	}
	if err := s.emit(ctx, shared.EventEmployeeRecordBulkUpserted, uuid.Nil, map[string]any{
		"row_count": n,
	}); err != nil {
		return BulkResult{}, err
	}
	return BulkResult{RowCount: n}, nil
}

// ListMappings returns provider-attribute mappings for an application.
func (s *Service) ListMappings(ctx context.Context, appID uuid.UUID) ([]*EmployeeProviderAttributeMapping, error) {
	if s.apps != nil {
		ok, err := s.apps.ApplicationExists(ctx, appID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrApplicationNotFound
		}
	}
	return s.repo.ListMappings(ctx, appID, nil, nil)
}

// CreateMapping inserts a per-application mapping row.
func (s *Service) CreateMapping(ctx context.Context, appID uuid.UUID, in MappingCreatePayload) (*EmployeeProviderAttributeMapping, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	if s.apps != nil {
		ok, err := s.apps.ApplicationExists(ctx, appID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrApplicationNotFound
		}
	}
	m := &EmployeeProviderAttributeMapping{
		ID:                s.idGen(),
		ApplicationID:     appID,
		EmployeeRecordKey: strings.TrimSpace(in.EmployeeRecordKey),
		PersonKey:         strings.TrimSpace(in.PersonKey),
		IsDeterminator:    in.IsDeterminator,
		AllowUpstream:     in.AllowUpstream,
	}
	if err := s.repo.InsertMapping(ctx, m); err != nil {
		return nil, translateMappingInsertError(err)
	}
	return m, nil
}

// DeleteMapping removes a per-application mapping row.
func (s *Service) DeleteMapping(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteMapping(ctx, id)
}

// ListMatches returns every EmployeeRecordMatch row in the system.
// Intended for clients that need to enrich a list of records with
// their resolved (person, employment) without N+1 lookups.
func (s *Service) ListMatches(ctx context.Context) ([]*EmployeeRecordMatch, error) {
	return s.repo.ListMatches(ctx)
}

// GetMatch returns the EmployeeRecordMatch for a record, or nil.
func (s *Service) GetMatch(ctx context.Context, recordID uuid.UUID) (*EmployeeRecordMatch, error) {
	if _, err := s.repo.GetRecordByID(ctx, recordID); err != nil {
		return nil, err
	}
	m, err := s.repo.GetMatchByRecord(ctx, recordID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	return m, nil
}

// SetMatch persists a manual match between a record and a
// (person, employment) pair. Emits inventory.employee_record.matched.
func (s *Service) SetMatch(ctx context.Context, recordID uuid.UUID, in MatchCreatePayload) (*EmployeeRecordMatch, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}
	if _, err := s.repo.GetRecordByID(ctx, recordID); err != nil {
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
	if s.employments != nil {
		ok, err := s.employments.EmploymentExistsForPerson(ctx, in.EmploymentID, in.PersonID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrEmploymentNotFound
		}
	}
	m := &EmployeeRecordMatch{
		ID:                     s.idGen(),
		EmployeeRecordID:       recordID,
		PersonID:               in.PersonID,
		EmploymentID:           in.EmploymentID,
		MatchedViaDeterminator: in.MatchedViaDeterminator,
	}
	if err := s.repo.UpsertMatch(ctx, m); err != nil {
		return nil, err
	}
	if err := s.emit(ctx, shared.EventEmployeeRecordMatched, recordID, map[string]any{
		"employee_record_id":       recordID.String(),
		"person_id":                in.PersonID.String(),
		"employment_id":            in.EmploymentID.String(),
		"matched_via_determinator": in.MatchedViaDeterminator,
		"manual":                   true,
	}); err != nil {
		return nil, err
	}
	return m, nil
}

// ClearMatch removes the EmployeeRecordMatch for a record and emits
// inventory.employee_record.unmatched.
func (s *Service) ClearMatch(ctx context.Context, recordID uuid.UUID) error {
	if _, err := s.repo.GetRecordByID(ctx, recordID); err != nil {
		return err
	}
	if err := s.repo.DeleteMatch(ctx, recordID); err != nil {
		return err
	}
	return s.emit(ctx, shared.EventEmployeeRecordUnmatched, recordID, map[string]any{
		"employee_record_id": recordID.String(),
	})
}

// ResolveAndPersist runs the Resolver for a record, upserts the
// resulting EmployeeRecordMatch (or clears it if no match was found),
// and emits inventory.employee_record.matched / .unmatched accordingly.
//
// Idempotent — calling repeatedly converges to the same state for a
// stable attribute set.
func (s *Service) ResolveAndPersist(ctx context.Context, recordID uuid.UUID) (ResolveResult, error) {
	if s.resolver == nil {
		return ResolveResult{}, fmt.Errorf("employee_records: resolver not wired")
	}
	if _, err := s.repo.GetRecordByID(ctx, recordID); err != nil {
		return ResolveResult{}, err
	}
	outcome, err := s.resolver.Resolve(ctx, recordID)
	if err != nil {
		return ResolveResult{}, err
	}
	if outcome == nil || outcome.PersonID == uuid.Nil {
		if err := s.repo.DeleteMatch(ctx, recordID); err != nil {
			return ResolveResult{}, err
		}
		if err := s.emit(ctx, shared.EventEmployeeRecordUnmatched, recordID, map[string]any{
			"employee_record_id": recordID.String(),
		}); err != nil {
			return ResolveResult{}, err
		}
		return ResolveResult{EmployeeRecordID: recordID, Resolved: false}, nil
	}
	m := &EmployeeRecordMatch{
		ID:                     s.idGen(),
		EmployeeRecordID:       recordID,
		PersonID:               outcome.PersonID,
		EmploymentID:           outcome.EmploymentID,
		MatchedViaDeterminator: outcome.ViaDeterminator,
	}
	if err := s.repo.UpsertMatch(ctx, m); err != nil {
		return ResolveResult{}, err
	}
	if err := s.emit(ctx, shared.EventEmployeeRecordMatched, recordID, map[string]any{
		"employee_record_id":       recordID.String(),
		"person_id":                outcome.PersonID.String(),
		"employment_id":            outcome.EmploymentID.String(),
		"matched_via_determinator": outcome.ViaDeterminator,
		"manual":                   false,
	}); err != nil {
		return ResolveResult{}, err
	}
	return ResolveResult{
		EmployeeRecordID:       recordID,
		PersonID:               &outcome.PersonID,
		EmploymentID:           &outcome.EmploymentID,
		MatchedViaDeterminator: outcome.ViaDeterminator,
		Resolved:               true,
	}, nil
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
		ActorID:       shared.EventActorComponentEmployeeRecords,
	}
	if target != uuid.Nil {
		in.TargetKind = events.ParticipantSystem
		in.TargetID = target.String()
	}
	env, err := events.NewEnvelope(in)
	if err != nil {
		return fmt.Errorf("employee_records: build event: %w", err)
	}
	if err := s.sink.Emit(ctx, env); err != nil {
		return fmt.Errorf("employee_records: emit event: %w", err)
	}
	return nil
}

func translateRecordInsertError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr pgdriver.Error
	if errors.As(err, &pgErr) {
		if pgErr.Field('C') == "23505" && strings.Contains(pgErr.Field('M'), "uq_employee_records_app_external_id") {
			return ErrDuplicate
		}
	}
	return err
}

func translateMappingInsertError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr pgdriver.Error
	if errors.As(err, &pgErr) {
		if pgErr.Field('C') == "23505" && strings.Contains(pgErr.Field('M'), "uq_eprov_attr_map_app_rec_key") {
			return ErrMappingDuplicate
		}
	}
	return err
}
