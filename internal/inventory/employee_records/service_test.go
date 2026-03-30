// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package employee_records

import (
	"context"
	"errors"
	"testing"

	"github.com/aurelion-solutions/backplane/internal/core/events"
	"github.com/google/uuid"
)

type recordingSink struct{ events []events.Envelope }

func (s *recordingSink) Emit(_ context.Context, env events.Envelope) error {
	s.events = append(s.events, env)
	return nil
}

type stubApps struct{ exists map[uuid.UUID]bool }

func (a *stubApps) ApplicationExists(_ context.Context, id uuid.UUID) (bool, error) {
	return a.exists[id], nil
}
func (a *stubApps) ApplicationIDByCode(_ context.Context, code string) (uuid.UUID, bool, error) {
	for id := range a.exists {
		if id.String() == code {
			return id, true, nil
		}
	}
	return uuid.Nil, false, nil
}

type stubPersons struct{ exists map[uuid.UUID]bool }

func (s *stubPersons) PersonExists(_ context.Context, id uuid.UUID) (bool, error) {
	return s.exists[id], nil
}

type stubEmployments struct {
	personByEmp map[uuid.UUID]uuid.UUID
}

func (s *stubEmployments) EmploymentExistsForPerson(_ context.Context, emp, person uuid.UUID) (bool, error) {
	pid, ok := s.personByEmp[emp]
	return ok && pid == person, nil
}

func newService(t *testing.T) (*Service, *fakeRepo, *recordingSink, *stubApps, *stubPersons, *stubEmployments, *fakePersonAPI) {
	t.Helper()
	repo := newFakeRepo()
	sink := &recordingSink{}
	apps := &stubApps{exists: map[uuid.UUID]bool{}}
	persons := &stubPersons{exists: map[uuid.UUID]bool{}}
	emps := &stubEmployments{personByEmp: map[uuid.UUID]uuid.UUID{}}
	personsAPI := newFakePersons()
	res := NewResolver(repo, personsAPI)
	svc := NewService(Deps{
		Repo:         repo,
		Sink:         sink,
		Apps:         apps,
		Persons:      persons,
		Employments:  emps,
		AppsResolver: apps,
		Resolver:     res,
	})
	return svc, repo, sink, apps, persons, emps, personsAPI
}

func TestCreateRecord_emitsEvent(t *testing.T) {
	svc, _, sink, apps, _, _, _ := newService(t)
	appID := uuid.New()
	apps.exists[appID] = true
	row, err := svc.CreateRecord(context.Background(), CreatePayload{ExternalID: "src-1", ApplicationID: appID})
	if err != nil {
		t.Fatal(err)
	}
	if row.ID == uuid.Nil {
		t.Fatal("expected ID")
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.employee_record.created" {
		t.Fatalf("expected created event, got %+v", sink.events)
	}
}

func TestCreateRecord_unknownApp(t *testing.T) {
	svc, _, _, _, _, _, _ := newService(t)
	_, err := svc.CreateRecord(context.Background(), CreatePayload{ExternalID: "src-1", ApplicationID: uuid.New()})
	if !errors.Is(err, ErrApplicationNotFound) {
		t.Fatalf("expected ErrApplicationNotFound, got %v", err)
	}
}

func TestCreateMapping_unknownApp(t *testing.T) {
	svc, _, _, _, _, _, _ := newService(t)
	_, err := svc.CreateMapping(context.Background(), uuid.New(), MappingCreatePayload{
		EmployeeRecordKey: "email_src", PersonKey: "email", IsDeterminator: true,
	})
	if !errors.Is(err, ErrApplicationNotFound) {
		t.Fatalf("expected ErrApplicationNotFound, got %v", err)
	}
}

func TestSetMatch_manual_emitsMatched(t *testing.T) {
	svc, repo, sink, apps, persons, emps, _ := newService(t)
	appID := uuid.New()
	apps.exists[appID] = true
	recordID := uuid.New()
	personID := uuid.New()
	employmentID := uuid.New()
	persons.exists[personID] = true
	emps.personByEmp[employmentID] = personID
	repo.records[recordID] = &EmployeeRecord{ID: recordID, ApplicationID: appID, ExternalID: "x"}
	sink.events = nil

	if _, err := svc.SetMatch(context.Background(), recordID, MatchCreatePayload{
		PersonID: personID, EmploymentID: employmentID, MatchedViaDeterminator: true,
	}); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.employee_record.matched" {
		t.Fatalf("expected matched, got %+v", sink.events)
	}
	if sink.events[0].Payload["manual"] != true {
		t.Fatalf("expected manual=true, got %v", sink.events[0].Payload["manual"])
	}
	if sink.events[0].Payload["person_id"] != personID.String() ||
		sink.events[0].Payload["employment_id"] != employmentID.String() {
		t.Fatalf("unexpected payload: %+v", sink.events[0].Payload)
	}
}

func TestSetMatch_rejectsUnknownPerson(t *testing.T) {
	svc, repo, _, apps, _, _, _ := newService(t)
	appID := uuid.New()
	apps.exists[appID] = true
	recordID := uuid.New()
	repo.records[recordID] = &EmployeeRecord{ID: recordID, ApplicationID: appID}
	_, err := svc.SetMatch(context.Background(), recordID, MatchCreatePayload{
		PersonID: uuid.New(), EmploymentID: uuid.New(),
	})
	if !errors.Is(err, ErrPersonNotFound) {
		t.Fatalf("expected ErrPersonNotFound, got %v", err)
	}
}

func TestSetMatch_rejectsEmploymentNotOwnedByPerson(t *testing.T) {
	svc, repo, _, apps, persons, emps, _ := newService(t)
	appID := uuid.New()
	apps.exists[appID] = true
	recordID := uuid.New()
	personID := uuid.New()
	employmentID := uuid.New()
	persons.exists[personID] = true
	// Employment exists for a DIFFERENT person.
	emps.personByEmp[employmentID] = uuid.New()
	repo.records[recordID] = &EmployeeRecord{ID: recordID, ApplicationID: appID}
	_, err := svc.SetMatch(context.Background(), recordID, MatchCreatePayload{
		PersonID: personID, EmploymentID: employmentID,
	})
	if !errors.Is(err, ErrEmploymentNotFound) {
		t.Fatalf("expected ErrEmploymentNotFound, got %v", err)
	}
}

func TestResolveAndPersist_matched_emitsMatched(t *testing.T) {
	svc, repo, sink, _, _, _, personsAPI := newService(t)
	appID := uuid.New()
	recordID := uuid.New()
	personID := uuid.New()
	employmentID := uuid.New()

	repo.records[recordID] = &EmployeeRecord{ID: recordID, ApplicationID: appID}
	repo.attrs[recordID] = map[string]string{"email_src": "alice@x.com"}
	repo.mappings = []*EmployeeProviderAttributeMapping{
		{ID: uuid.New(), ApplicationID: appID, EmployeeRecordKey: "email_src", PersonKey: "email", IsDeterminator: true},
	}
	personsAPI.keyValueIndex["email|alice@x.com"] = personID
	personsAPI.employmentForPerson[personID] = employmentID
	sink.events = nil

	res, err := svc.ResolveAndPersist(context.Background(), recordID)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Resolved || res.PersonID == nil || *res.PersonID != personID ||
		res.EmploymentID == nil || *res.EmploymentID != employmentID {
		t.Fatalf("expected matched result, got %+v", res)
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.employee_record.matched" {
		t.Fatalf("expected matched, got %+v", sink.events)
	}
	if sink.events[0].Payload["manual"] != false {
		t.Fatalf("expected manual=false, got %v", sink.events[0].Payload["manual"])
	}
}

func TestResolveAndPersist_unresolved_emitsUnmatched(t *testing.T) {
	svc, repo, sink, _, _, _, _ := newService(t)
	appID := uuid.New()
	recordID := uuid.New()
	repo.records[recordID] = &EmployeeRecord{ID: recordID, ApplicationID: appID}
	repo.attrs[recordID] = map[string]string{"x": "y"}
	sink.events = nil

	res, err := svc.ResolveAndPersist(context.Background(), recordID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Resolved {
		t.Fatalf("expected unresolved, got %+v", res)
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.employee_record.unmatched" {
		t.Fatalf("expected unmatched, got %+v", sink.events)
	}
}

func TestBulkUpsert_emitsBulkEvent(t *testing.T) {
	svc, _, sink, apps, _, _, _ := newService(t)
	appID := uuid.New()
	apps.exists[appID] = true
	res, err := svc.BulkUpsert(context.Background(), BulkPayload{Items: []BulkItem{
		{ApplicationCode: appID.String(), ExternalID: "src-1"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	_ = res
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.employee_record.bulk_upserted" {
		t.Fatalf("expected bulk_upserted, got %+v", sink.events)
	}
}
