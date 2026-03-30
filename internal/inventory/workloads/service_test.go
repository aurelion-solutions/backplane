// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package workloads

import (
	"context"
	"errors"
	"testing"

	"github.com/aurelion-solutions/backplane/internal/core/events"
	"github.com/google/uuid"
)

type memRepo struct {
	rows  map[uuid.UUID]*Workload
	attrs map[uuid.UUID]map[string]*WorkloadAttribute
}

func newMemRepo() *memRepo {
	return &memRepo{
		rows:  map[uuid.UUID]*Workload{},
		attrs: map[uuid.UUID]map[string]*WorkloadAttribute{},
	}
}

func (r *memRepo) GetByID(_ context.Context, id uuid.UUID) (*Workload, error) {
	if w, ok := r.rows[id]; ok {
		return w, nil
	}
	return nil, ErrNotFound
}
func (r *memRepo) GetByExternalID(_ context.Context, ext string) (*Workload, error) {
	for _, w := range r.rows {
		if w.ExternalID == ext {
			return w, nil
		}
	}
	return nil, ErrNotFound
}
func (r *memRepo) List(_ context.Context, limit, offset int) ([]*Workload, int, error) {
	out := []*Workload{}
	for _, w := range r.rows {
		out = append(out, w)
	}
	total := len(out)
	if offset >= total {
		return []*Workload{}, total, nil
	}
	out = out[offset:]
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, total, nil
}
func (r *memRepo) Insert(_ context.Context, w *Workload) error {
	for _, ex := range r.rows {
		if ex.ExternalID == w.ExternalID {
			return ErrExternalIDAlreadyExists
		}
	}
	r.rows[w.ID] = w
	return nil
}
func (r *memRepo) Update(_ context.Context, w *Workload) error {
	r.rows[w.ID] = w
	return nil
}
func (r *memRepo) ListAttributes(_ context.Context, id uuid.UUID) ([]*WorkloadAttribute, error) {
	out := []*WorkloadAttribute{}
	for _, a := range r.attrs[id] {
		out = append(out, a)
	}
	return out, nil
}
func (r *memRepo) GetAttribute(_ context.Context, id uuid.UUID, key string) (*WorkloadAttribute, error) {
	if a, ok := r.attrs[id][key]; ok {
		return a, nil
	}
	return nil, ErrAttributeNotFound
}
func (r *memRepo) UpsertAttribute(_ context.Context, a *WorkloadAttribute) error {
	if r.attrs[a.WorkloadID] == nil {
		r.attrs[a.WorkloadID] = map[string]*WorkloadAttribute{}
	}
	r.attrs[a.WorkloadID][a.Key] = a
	return nil
}
func (r *memRepo) DeleteAttribute(_ context.Context, id uuid.UUID, key string) error {
	if _, ok := r.attrs[id][key]; !ok {
		return ErrAttributeNotFound
	}
	delete(r.attrs[id], key)
	return nil
}
func (r *memRepo) BulkUpsert(_ context.Context, items []BulkItem, idGen func() uuid.UUID) (int, error) {
	for _, it := range items {
		var existing *Workload
		for _, w := range r.rows {
			if w.ExternalID == it.ExternalID {
				existing = w
				break
			}
		}
		if existing == nil {
			existing = &Workload{ID: idGen(), ExternalID: it.ExternalID}
			r.rows[existing.ID] = existing
		}
		existing.Name = it.Name
		existing.Description = it.Description
		existing.OwnerEmploymentID = it.OwnerEmploymentID
		existing.ApplicationID = it.ApplicationID
	}
	return len(items), nil
}

type recordingSink struct{ events []events.Envelope }

func (s *recordingSink) Emit(_ context.Context, env events.Envelope) error {
	s.events = append(s.events, env)
	return nil
}

type stubEmployments struct{ exists map[uuid.UUID]bool }

func (e *stubEmployments) EmploymentExists(_ context.Context, id uuid.UUID) (bool, error) {
	return e.exists[id], nil
}

type stubApps struct{ exists map[uuid.UUID]bool }

func (a *stubApps) ApplicationExists(_ context.Context, id uuid.UUID) (bool, error) {
	return a.exists[id], nil
}

func newService(t *testing.T) (*Service, *memRepo, *recordingSink, *stubEmployments, *stubApps) {
	t.Helper()
	repo := newMemRepo()
	sink := &recordingSink{}
	emp := &stubEmployments{exists: map[uuid.UUID]bool{}}
	apps := &stubApps{exists: map[uuid.UUID]bool{}}
	svc := NewService(Deps{Repo: repo, Sink: sink, Employments: emp, Apps: apps})
	return svc, repo, sink, emp, apps
}

func TestCreate_emitsEvent(t *testing.T) {
	svc, _, sink, _, _ := newService(t)
	w, err := svc.Create(context.Background(), CreatePayload{ExternalID: "wl-1", Name: "ETL"})
	if err != nil {
		t.Fatal(err)
	}
	if w.ID == uuid.Nil {
		t.Fatal("expected ID")
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.workload.created" {
		t.Fatalf("expected created event, got %+v", sink.events)
	}
}

func TestCreate_duplicateExternalID(t *testing.T) {
	svc, _, _, _, _ := newService(t)
	_, _ = svc.Create(context.Background(), CreatePayload{ExternalID: "dup", Name: "x"})
	_, err := svc.Create(context.Background(), CreatePayload{ExternalID: "dup", Name: "y"})
	if !errors.Is(err, ErrExternalIDAlreadyExists) {
		t.Fatalf("expected ErrExternalIDAlreadyExists, got %v", err)
	}
}

func TestCreate_ownerMustExist(t *testing.T) {
	svc, _, _, _, _ := newService(t)
	bad := uuid.New()
	_, err := svc.Create(context.Background(), CreatePayload{
		ExternalID: "wl-1", Name: "ETL", OwnerEmploymentID: &bad,
	})
	if !errors.Is(err, ErrOwnerNotFound) {
		t.Fatalf("expected ErrOwnerNotFound, got %v", err)
	}
}

func TestCreate_applicationMustExist(t *testing.T) {
	svc, _, _, _, _ := newService(t)
	bad := uuid.New()
	_, err := svc.Create(context.Background(), CreatePayload{
		ExternalID: "wl-1", Name: "ETL", ApplicationID: &bad,
	})
	if !errors.Is(err, ErrApplicationNotFound) {
		t.Fatalf("expected ErrApplicationNotFound, got %v", err)
	}
}

func TestUpdate_ownerEmployment_changeEmitsChanges(t *testing.T) {
	svc, _, sink, emp, _ := newService(t)
	w, _ := svc.Create(context.Background(), CreatePayload{ExternalID: "wl-1", Name: "ETL"})
	owner := uuid.New()
	emp.exists[owner] = true
	sink.events = nil

	if _, err := svc.Update(context.Background(), w.ID, PatchPayload{OwnerEmploymentID: &owner}); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.workload.updated" {
		t.Fatalf("expected updated, got %+v", sink.events)
	}
	changes, _ := sink.events[0].Payload["changes"].(map[string]any)
	if changes["owner_employment_id"] == nil {
		t.Fatalf("expected owner_employment_id in changes, got %+v", changes)
	}
}

func TestBulkUpsert_emitsBulkEvent(t *testing.T) {
	svc, _, sink, _, _ := newService(t)
	res, err := svc.BulkUpsert(context.Background(), BulkPayload{Items: []BulkItem{
		{ExternalID: "wl-1", Name: "A"},
		{ExternalID: "wl-2", Name: "B"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if res.RowCount != 2 {
		t.Fatalf("expected 2, got %d", res.RowCount)
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.workload.bulk_upserted" {
		t.Fatalf("expected bulk_upserted, got %+v", sink.events)
	}
}
