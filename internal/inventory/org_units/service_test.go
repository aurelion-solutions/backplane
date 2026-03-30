// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package org_units

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/events"
	"github.com/google/uuid"
)

type memRepo struct {
	rows map[uuid.UUID]*OrgUnit
}

func newMemRepo() *memRepo { return &memRepo{rows: map[uuid.UUID]*OrgUnit{}} }

func (r *memRepo) GetByID(_ context.Context, id uuid.UUID) (*OrgUnit, error) {
	if u, ok := r.rows[id]; ok {
		return u, nil
	}
	return nil, ErrNotFound
}
func (r *memRepo) GetByExternalID(_ context.Context, externalID string) (*OrgUnit, error) {
	for _, u := range r.rows {
		if u.ExternalID == externalID {
			return u, nil
		}
	}
	return nil, ErrNotFound
}
func (r *memRepo) List(_ context.Context, limit, offset int) ([]*OrgUnit, int, error) {
	out := []*OrgUnit{}
	for _, u := range r.rows {
		out = append(out, u)
	}
	total := len(out)
	if offset >= total {
		return []*OrgUnit{}, total, nil
	}
	out = out[offset:]
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, total, nil
}
func (r *memRepo) Insert(_ context.Context, u *OrgUnit) error {
	for _, ex := range r.rows {
		if ex.ExternalID == u.ExternalID {
			return errors.New("uq_org_units_external_id: simulated")
		}
	}
	r.rows[u.ID] = u
	return nil
}
func (r *memRepo) Update(_ context.Context, u *OrgUnit) error {
	r.rows[u.ID] = u
	return nil
}
func (r *memRepo) Delete(_ context.Context, id uuid.UUID) error {
	delete(r.rows, id)
	return nil
}
func (r *memRepo) BulkUpsert(_ context.Context, items []BulkItem, idGen func() uuid.UUID) (int, error) {
	ids := map[string]uuid.UUID{}
	for _, it := range items {
		var existing *OrgUnit
		for _, u := range r.rows {
			if u.ExternalID == it.ExternalID {
				existing = u
				break
			}
		}
		if existing == nil {
			existing = &OrgUnit{ID: idGen(), ExternalID: it.ExternalID, IsInternal: false}
			r.rows[existing.ID] = existing
		}
		existing.Name = it.Name
		existing.Description = it.Description
		ids[it.ExternalID] = existing.ID
	}
	for _, it := range items {
		row := r.rows[ids[it.ExternalID]]
		if it.ParentExternalID == nil {
			row.ParentID = nil
			continue
		}
		pid, ok := ids[*it.ParentExternalID]
		if !ok {
			return 0, ErrParentNotFound
		}
		row.ParentID = &pid
	}
	return len(items), nil
}

type recordingSink struct{ events []events.Envelope }

func (s *recordingSink) Emit(_ context.Context, env events.Envelope) error {
	s.events = append(s.events, env)
	return nil
}

func newService(t *testing.T) (*Service, *memRepo, *recordingSink) {
	t.Helper()
	repo := newMemRepo()
	sink := &recordingSink{}
	now := func() time.Time { return time.Unix(1700000000, 0).UTC() }
	return NewService(repo, sink, uuid.New, now), repo, sink
}

func TestCreate_externalOnly_emitsEvent(t *testing.T) {
	svc, repo, sink := newService(t)
	u, err := svc.Create(context.Background(), CreatePayload{ExternalID: "eng", Name: "Engineering"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.IsInternal {
		t.Fatal("expected is_internal=false on API-created node")
	}
	if _, ok := repo.rows[u.ID]; !ok {
		t.Fatal("not persisted")
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.org_unit.created" {
		t.Fatalf("expected created event, got %+v", sink.events)
	}
}

func TestCreate_rejectsInternalParent(t *testing.T) {
	svc, repo, _ := newService(t)
	parentID := uuid.New()
	repo.rows[parentID] = &OrgUnit{ID: parentID, ExternalID: "root", Name: "Root", IsInternal: true}
	_, err := svc.Create(context.Background(), CreatePayload{ExternalID: "child", Name: "Child", ParentID: &parentID})
	if !errors.Is(err, ErrParentInternal) {
		t.Fatalf("expected ErrParentInternal, got %v", err)
	}
}

func TestCreate_parentNotFound(t *testing.T) {
	svc, _, _ := newService(t)
	missing := uuid.New()
	_, err := svc.Create(context.Background(), CreatePayload{ExternalID: "x", Name: "Y", ParentID: &missing})
	if !errors.Is(err, ErrParentNotFound) {
		t.Fatalf("expected ErrParentNotFound, got %v", err)
	}
}

func TestUpdate_rejectsInternal(t *testing.T) {
	svc, repo, _ := newService(t)
	id := uuid.New()
	repo.rows[id] = &OrgUnit{ID: id, ExternalID: "root", Name: "Root", IsInternal: true}
	name := "renamed"
	_, err := svc.Update(context.Background(), id, PatchPayload{Name: &name})
	if !errors.Is(err, ErrCannotDeleteInternal) {
		t.Fatalf("expected internal-read-only error, got %v", err)
	}
}

func TestUpdate_emitsChangesEvent(t *testing.T) {
	svc, _, sink := newService(t)
	u, _ := svc.Create(context.Background(), CreatePayload{ExternalID: "eng", Name: "Engineering"})
	sink.events = nil
	name := "Engineering Ops"
	if _, err := svc.Update(context.Background(), u.ID, PatchPayload{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.org_unit.updated" {
		t.Fatalf("expected updated event, got %+v", sink.events)
	}
	changes, ok := sink.events[0].Payload["changes"].(map[string]any)
	if !ok || changes["name"] == nil {
		t.Fatalf("expected changes.name in payload, got %+v", sink.events[0].Payload)
	}
}

func TestUpdate_noFieldsRejected(t *testing.T) {
	svc, _, _ := newService(t)
	u, _ := svc.Create(context.Background(), CreatePayload{ExternalID: "eng", Name: "Engineering"})
	if _, err := svc.Update(context.Background(), u.ID, PatchPayload{}); !errors.Is(err, ErrNoFields) {
		t.Fatalf("expected ErrNoFields, got %v", err)
	}
}

func TestDelete_rejectsInternal_emitsOnExternal(t *testing.T) {
	svc, repo, sink := newService(t)
	internalID := uuid.New()
	repo.rows[internalID] = &OrgUnit{ID: internalID, ExternalID: "root", IsInternal: true}
	if err := svc.Delete(context.Background(), internalID); !errors.Is(err, ErrCannotDeleteInternal) {
		t.Fatalf("expected internal-read-only error, got %v", err)
	}

	u, _ := svc.Create(context.Background(), CreatePayload{ExternalID: "eng", Name: "Engineering"})
	sink.events = nil
	if err := svc.Delete(context.Background(), u.ID); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.org_unit.deleted" {
		t.Fatalf("expected deleted event, got %+v", sink.events)
	}
}

func TestBulkUpsert_resolvesParentByExternalID(t *testing.T) {
	svc, _, sink := newService(t)
	res, err := svc.BulkUpsert(context.Background(), BulkPayload{Items: []BulkItem{
		{ExternalID: "root", Name: "Root"},
		{ExternalID: "child", Name: "Child", ParentExternalID: strPtr("root")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if res.RowCount != 2 {
		t.Fatalf("expected 2, got %d", res.RowCount)
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.org_unit.bulk_upserted" {
		t.Fatalf("expected bulk_upserted, got %+v", sink.events)
	}
}

func TestBulkUpsert_rejectsSelfRefAtSchemaLayer(t *testing.T) {
	svc, _, _ := newService(t)
	_, err := svc.BulkUpsert(context.Background(), BulkPayload{Items: []BulkItem{
		{ExternalID: "self", Name: "x", ParentExternalID: strPtr("self")},
	}})
	if err == nil {
		t.Fatal("expected self-reference error")
	}
}

func strPtr(s string) *string { return &s }
