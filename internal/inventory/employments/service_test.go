// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package employments

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/events"
	"github.com/aurelion-solutions/backplane/internal/inventory/shared"
	"github.com/google/uuid"
)

type memRepo struct {
	rows  map[uuid.UUID]*Employment
	attrs map[uuid.UUID]map[string]*EmploymentAttribute
}

func newMemRepo() *memRepo {
	return &memRepo{
		rows:  map[uuid.UUID]*Employment{},
		attrs: map[uuid.UUID]map[string]*EmploymentAttribute{},
	}
}

func (r *memRepo) GetByID(_ context.Context, id uuid.UUID) (*Employment, error) {
	if e, ok := r.rows[id]; ok {
		return e, nil
	}
	return nil, ErrNotFound
}
func (r *memRepo) ListByPerson(_ context.Context, pid uuid.UUID) ([]*Employment, error) {
	out := []*Employment{}
	for _, e := range r.rows {
		if e.PersonID == pid {
			out = append(out, e)
		}
	}
	return out, nil
}
func (r *memRepo) ListActiveByPerson(_ context.Context, pid uuid.UUID, at time.Time) ([]*Employment, error) {
	out := []*Employment{}
	for _, e := range r.rows {
		if e.PersonID != pid {
			continue
		}
		if e.IsActiveAt(at) {
			out = append(out, e)
		}
	}
	return out, nil
}
func (r *memRepo) List(_ context.Context, limit, offset int) ([]*Employment, int, error) {
	out := []*Employment{}
	for _, e := range r.rows {
		out = append(out, e)
	}
	total := len(out)
	if offset >= total {
		return []*Employment{}, total, nil
	}
	out = out[offset:]
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, total, nil
}
func (r *memRepo) Insert(_ context.Context, e *Employment) error {
	r.rows[e.ID] = e
	return nil
}
func (r *memRepo) Update(_ context.Context, e *Employment) error {
	r.rows[e.ID] = e
	return nil
}
func (r *memRepo) ListAttributes(_ context.Context, id uuid.UUID) ([]*EmploymentAttribute, error) {
	out := []*EmploymentAttribute{}
	for _, a := range r.attrs[id] {
		out = append(out, a)
	}
	return out, nil
}
func (r *memRepo) GetAttribute(_ context.Context, id uuid.UUID, key string) (*EmploymentAttribute, error) {
	if a, ok := r.attrs[id][key]; ok {
		return a, nil
	}
	return nil, ErrAttributeNotFound
}
func (r *memRepo) UpsertAttribute(_ context.Context, a *EmploymentAttribute) error {
	if r.attrs[a.EmploymentID] == nil {
		r.attrs[a.EmploymentID] = map[string]*EmploymentAttribute{}
	}
	r.attrs[a.EmploymentID][a.Key] = a
	return nil
}
func (r *memRepo) DeleteAttribute(_ context.Context, id uuid.UUID, key string) error {
	if _, ok := r.attrs[id][key]; !ok {
		return ErrAttributeNotFound
	}
	delete(r.attrs[id], key)
	return nil
}
func (r *memRepo) BulkUpsert(_ context.Context, items []BulkItem, persons PersonResolver, _ OrgUnitResolver, idGen func() uuid.UUID) (int, error) {
	for _, it := range items {
		pid, ok, _ := persons.PersonIDByExternalID(context.Background(), it.PersonExternalID)
		if !ok {
			return 0, ErrPersonNotFound
		}
		var existing *Employment
		for _, e := range r.rows {
			if e.PersonID == pid && e.Code == it.Code && e.StartDate.Equal(it.StartDate) {
				existing = e
				break
			}
		}
		if existing == nil {
			existing = &Employment{ID: idGen(), PersonID: pid, Code: it.Code, StartDate: it.StartDate}
			r.rows[existing.ID] = existing
		}
		existing.EndDate = it.EndDate
	}
	return len(items), nil
}

type recordingSink struct{ events []events.Envelope }

func (s *recordingSink) Emit(_ context.Context, env events.Envelope) error {
	s.events = append(s.events, env)
	return nil
}

type stubPersons struct{ exists map[uuid.UUID]bool }

func (p *stubPersons) PersonExists(_ context.Context, id uuid.UUID) (bool, error) {
	return p.exists[id], nil
}
func (p *stubPersons) PersonIDByExternalID(_ context.Context, ext string) (uuid.UUID, bool, error) {
	for id := range p.exists {
		if id.String() == ext {
			return id, true, nil
		}
	}
	return uuid.Nil, false, nil
}

type stubOrgUnits struct{ exists map[uuid.UUID]bool }

func (o *stubOrgUnits) OrgUnitExists(_ context.Context, id uuid.UUID) (bool, error) {
	return o.exists[id], nil
}
func (o *stubOrgUnits) OrgUnitIDByExternalID(_ context.Context, _ string) (uuid.UUID, bool, error) {
	return uuid.Nil, false, nil
}

type stubRecomputer struct {
	calls []struct {
		kind shared.PrincipalKind
		id   uuid.UUID
	}
}

func (s *stubRecomputer) RecomputeForBody(_ context.Context, kind shared.PrincipalKind, id uuid.UUID) error {
	s.calls = append(s.calls, struct {
		kind shared.PrincipalKind
		id   uuid.UUID
	}{kind, id})
	return nil
}

func newService(t *testing.T) (*Service, *memRepo, *recordingSink, *stubPersons, *stubOrgUnits, *stubRecomputer) {
	t.Helper()
	repo := newMemRepo()
	sink := &recordingSink{}
	persons := &stubPersons{exists: map[uuid.UUID]bool{}}
	orgUnits := &stubOrgUnits{exists: map[uuid.UUID]bool{}}
	rec := &stubRecomputer{}
	svc := NewService(Deps{
		Repo:            repo,
		Sink:            sink,
		Persons:         persons,
		OrgUnits:        orgUnits,
		Recomputer:      rec,
		PersonResolver:  persons,
		OrgUnitResolver: orgUnits,
		Now:             func() time.Time { return time.Unix(1700000000, 0).UTC() },
	})
	return svc, repo, sink, persons, orgUnits, rec
}

func TestCreate_emitsEvent(t *testing.T) {
	svc, _, sink, persons, _, _ := newService(t)
	pid := uuid.New()
	persons.exists[pid] = true
	start := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	e, err := svc.Create(context.Background(), CreatePayload{
		PersonID: pid, Code: "active", StartDate: start,
	})
	if err != nil {
		t.Fatal(err)
	}
	if e.Code != "active" || e.PersonID != pid {
		t.Fatalf("unexpected employment: %+v", e)
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.employment.created" {
		t.Fatalf("expected created, got %+v", sink.events)
	}
}

func TestCreate_arbitraryCodeAccepted(t *testing.T) {
	svc, _, _, persons, _, _ := newService(t)
	pid := uuid.New()
	persons.exists[pid] = true
	start := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	for _, code := range []string{"active", "probation", "maternity_leave", "notice_period", "x"} {
		if _, err := svc.Create(context.Background(), CreatePayload{
			PersonID: pid, Code: code, StartDate: start,
		}); err != nil {
			t.Fatalf("expected %q to be accepted, got %v", code, err)
		}
	}
}

func TestCreate_rejectsUnknownPerson(t *testing.T) {
	svc, _, _, _, _, _ := newService(t)
	start := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	_, err := svc.Create(context.Background(), CreatePayload{
		PersonID: uuid.New(), Code: "active", StartDate: start,
	})
	if !errors.Is(err, ErrPersonNotFound) {
		t.Fatalf("expected ErrPersonNotFound, got %v", err)
	}
}

func TestCreate_rejectsBadDates(t *testing.T) {
	svc, _, _, persons, _, _ := newService(t)
	pid := uuid.New()
	persons.exists[pid] = true
	start := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := svc.Create(context.Background(), CreatePayload{
		PersonID: pid, Code: "active", StartDate: start, EndDate: &end,
	})
	if !errors.Is(err, ErrInvalidDates) {
		t.Fatalf("expected ErrInvalidDates, got %v", err)
	}
}

func TestUpdate_codeChange_triggersRecompute(t *testing.T) {
	svc, _, sink, persons, _, rec := newService(t)
	pid := uuid.New()
	persons.exists[pid] = true
	start := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	e, _ := svc.Create(context.Background(), CreatePayload{PersonID: pid, Code: "active", StartDate: start})
	sink.events = nil
	rec.calls = nil
	newCode := "maternity_leave"
	if _, err := svc.Update(context.Background(), e.ID, PatchPayload{Code: &newCode}); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 1 || rec.calls[0].kind != shared.PrincipalKindEmployment || rec.calls[0].id != e.ID {
		t.Fatalf("expected employment recompute, got %+v", rec.calls)
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.employment.updated" {
		t.Fatalf("expected updated event, got %+v", sink.events)
	}
}

func TestUpdate_nonCodeChange_noRecompute(t *testing.T) {
	svc, _, _, persons, _, rec := newService(t)
	pid := uuid.New()
	persons.exists[pid] = true
	start := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	e, _ := svc.Create(context.Background(), CreatePayload{PersonID: pid, Code: "active", StartDate: start})
	rec.calls = nil
	desc := "Updated description"
	if _, err := svc.Update(context.Background(), e.ID, PatchPayload{Description: &desc}); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("expected no recompute on description-only change, got %+v", rec.calls)
	}
}

func TestEnd_setsEndDateEmitsEnded(t *testing.T) {
	svc, _, sink, persons, _, rec := newService(t)
	pid := uuid.New()
	persons.exists[pid] = true
	start := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	e, _ := svc.Create(context.Background(), CreatePayload{PersonID: pid, Code: "active", StartDate: start})
	sink.events = nil
	rec.calls = nil

	end := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	got, err := svc.End(context.Background(), e.ID, EndPayload{EndDate: end})
	if err != nil {
		t.Fatal(err)
	}
	if got.EndDate == nil || !got.EndDate.Equal(end) {
		t.Fatalf("expected end_date set, got %+v", got)
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.employment.ended" {
		t.Fatalf("expected ended event, got %+v", sink.events)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("expected recompute after end, got %+v", rec.calls)
	}
}

func TestEnd_rejectsAlreadyEnded(t *testing.T) {
	svc, _, _, persons, _, _ := newService(t)
	pid := uuid.New()
	persons.exists[pid] = true
	start := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	e, _ := svc.Create(context.Background(), CreatePayload{PersonID: pid, Code: "active", StartDate: start, EndDate: &end})
	end2 := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	_, err := svc.End(context.Background(), e.ID, EndPayload{EndDate: end2})
	if !errors.Is(err, ErrAlreadyEnded) {
		t.Fatalf("expected ErrAlreadyEnded, got %v", err)
	}
}

func TestListActiveByPerson_filtersByDate(t *testing.T) {
	svc, _, _, persons, _, _ := newService(t)
	pid := uuid.New()
	persons.exists[pid] = true
	// Active developer: 2024-01-01 → open
	_, _ = svc.Create(context.Background(), CreatePayload{
		PersonID: pid, Code: "developer",
		StartDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	// Already-ended QA: 2023-01-01 → 2023-12-31
	endQA := time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC)
	_, _ = svc.Create(context.Background(), CreatePayload{
		PersonID: pid, Code: "qa",
		StartDate: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   &endQA,
	})

	at := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	active, err := svc.ListActiveByPerson(context.Background(), pid, at)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].Code != "developer" {
		t.Fatalf("expected only developer active, got %+v", active)
	}
}

func TestPerson_canHoldMultipleConcurrentEmployments(t *testing.T) {
	svc, _, _, persons, _, _ := newService(t)
	pid := uuid.New()
	persons.exists[pid] = true
	// developer mask, full-time
	_, err := svc.Create(context.Background(), CreatePayload{
		PersonID: pid, Code: "developer",
		StartDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	// QA mask, half-time, same person, same window
	_, err = svc.Create(context.Background(), CreatePayload{
		PersonID: pid, Code: "qa",
		StartDate: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	active, _ := svc.ListActiveByPerson(context.Background(), pid, at)
	if len(active) != 2 {
		t.Fatalf("expected 2 concurrent employments, got %d: %+v", len(active), active)
	}
}

func TestBulkUpsert_emitsBulkEvent(t *testing.T) {
	svc, _, sink, persons, _, _ := newService(t)
	pid := uuid.New()
	persons.exists[pid] = true
	start := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	res, err := svc.BulkUpsert(context.Background(), BulkPayload{Items: []BulkItem{
		{PersonExternalID: pid.String(), Code: "active", StartDate: start},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if res.RowCount != 1 {
		t.Fatalf("expected 1, got %d", res.RowCount)
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.employment.bulk_upserted" {
		t.Fatalf("expected bulk_upserted, got %+v", sink.events)
	}
}
