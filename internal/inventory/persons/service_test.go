// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package persons

import (
	"context"
	"errors"
	"testing"

	"github.com/aurelion-solutions/backplane/internal/core/events"
	"github.com/google/uuid"
)

type memRepo struct {
	rows  map[uuid.UUID]*Person
	attrs map[uuid.UUID]map[string]*PersonAttribute
}

func newMemRepo() *memRepo {
	return &memRepo{
		rows:  map[uuid.UUID]*Person{},
		attrs: map[uuid.UUID]map[string]*PersonAttribute{},
	}
}

func (r *memRepo) GetByID(_ context.Context, id uuid.UUID) (*Person, error) {
	if p, ok := r.rows[id]; ok {
		return p, nil
	}
	return nil, ErrNotFound
}
func (r *memRepo) GetByExternalID(_ context.Context, externalID string) (*Person, error) {
	for _, p := range r.rows {
		if p.ExternalID == externalID {
			return p, nil
		}
	}
	return nil, ErrNotFound
}
func (r *memRepo) List(_ context.Context, limit, offset int) ([]*Person, int, error) {
	out := []*Person{}
	for _, p := range r.rows {
		out = append(out, p)
	}
	total := len(out)
	if offset >= total {
		return []*Person{}, total, nil
	}
	out = out[offset:]
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, total, nil
}
func (r *memRepo) Insert(_ context.Context, p *Person) error {
	for _, existing := range r.rows {
		if existing.ExternalID == p.ExternalID {
			return errors.New("uq_persons_external_id: simulated")
		}
	}
	r.rows[p.ID] = p
	return nil
}
func (r *memRepo) ListAttributes(_ context.Context, personID uuid.UUID) ([]*PersonAttribute, error) {
	out := []*PersonAttribute{}
	for _, a := range r.attrs[personID] {
		out = append(out, a)
	}
	return out, nil
}
func (r *memRepo) GetAttribute(_ context.Context, personID uuid.UUID, key string) (*PersonAttribute, error) {
	if a, ok := r.attrs[personID][key]; ok {
		return a, nil
	}
	return nil, ErrAttributeNotFound
}
func (r *memRepo) UpsertAttribute(_ context.Context, a *PersonAttribute) error {
	if r.attrs[a.PersonID] == nil {
		r.attrs[a.PersonID] = map[string]*PersonAttribute{}
	}
	r.attrs[a.PersonID][a.Key] = a
	return nil
}
func (r *memRepo) DeleteAttribute(_ context.Context, personID uuid.UUID, key string) error {
	if _, ok := r.attrs[personID][key]; !ok {
		return ErrAttributeNotFound
	}
	delete(r.attrs[personID], key)
	return nil
}
func (r *memRepo) BulkUpsert(_ context.Context, items []BulkItem, idGen func() uuid.UUID) (int, error) {
	for _, it := range items {
		var existing *Person
		for _, p := range r.rows {
			if p.ExternalID == it.ExternalID {
				existing = p
				break
			}
		}
		if existing == nil {
			existing = &Person{ID: idGen(), ExternalID: it.ExternalID, FullName: it.FullName}
			r.rows[existing.ID] = existing
		} else {
			existing.FullName = it.FullName
		}
		if r.attrs[existing.ID] == nil {
			r.attrs[existing.ID] = map[string]*PersonAttribute{}
		}
		for k, v := range it.Attributes {
			r.attrs[existing.ID][k] = &PersonAttribute{ID: idGen(), PersonID: existing.ID, Key: k, Value: v}
		}
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
	idGen := func() uuid.UUID { return uuid.New() }
	return NewService(repo, sink, idGen), repo, sink
}

func TestCreate_emitsEventAndPersists(t *testing.T) {
	svc, repo, sink := newService(t)
	p, err := svc.Create(context.Background(), CreatePayload{ExternalID: "ext-1", FullName: "Alice"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.ID == uuid.Nil || p.ExternalID != "ext-1" || p.FullName != "Alice" {
		t.Fatalf("unexpected person: %+v", p)
	}
	if _, ok := repo.rows[p.ID]; !ok {
		t.Fatal("person not in repo")
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.person.created" {
		t.Fatalf("expected one created event, got %+v", sink.events)
	}
}

func TestCreate_duplicateExternalID(t *testing.T) {
	svc, _, _ := newService(t)
	if _, err := svc.Create(context.Background(), CreatePayload{ExternalID: "dup", FullName: "x"}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Create(context.Background(), CreatePayload{ExternalID: "dup", FullName: "y"})
	if err == nil {
		t.Fatal("expected duplicate error, got nil")
	}
}

func TestCreate_invalidPayload(t *testing.T) {
	svc, _, _ := newService(t)
	if _, err := svc.Create(context.Background(), CreatePayload{ExternalID: "", FullName: "x"}); err == nil {
		t.Fatal("expected validation error for empty external_id")
	}
	if _, err := svc.Create(context.Background(), CreatePayload{ExternalID: "ok", FullName: ""}); err == nil {
		t.Fatal("expected validation error for empty full_name")
	}
}

func TestAddAttribute_emitsEvent(t *testing.T) {
	svc, _, sink := newService(t)
	p, _ := svc.Create(context.Background(), CreatePayload{ExternalID: "ext-1", FullName: "Alice"})
	sink.events = nil

	a, err := svc.AddAttribute(context.Background(), p.ID, AttributeCreatePayload{Key: "email", Value: "a@b"})
	if err != nil {
		t.Fatalf("AddAttribute: %v", err)
	}
	if a.Key != "email" || a.Value != "a@b" {
		t.Fatalf("unexpected attribute: %+v", a)
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.person.attribute_added" {
		t.Fatalf("expected attribute_added event, got %+v", sink.events)
	}
}

func TestAddAttribute_idempotentByKey(t *testing.T) {
	svc, _, _ := newService(t)
	p, _ := svc.Create(context.Background(), CreatePayload{ExternalID: "ext-1", FullName: "Alice"})
	if _, err := svc.AddAttribute(context.Background(), p.ID, AttributeCreatePayload{Key: "k", Value: "v1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddAttribute(context.Background(), p.ID, AttributeCreatePayload{Key: "k", Value: "v2"}); err != nil {
		t.Fatal(err)
	}
	attrs, _ := svc.ListAttributes(context.Background(), p.ID)
	if len(attrs) != 1 || attrs[0].Value != "v2" {
		t.Fatalf("expected single upserted attribute v2, got %+v", attrs)
	}
}

func TestRemoveAttribute_emitsEvent_andNotFound(t *testing.T) {
	svc, _, sink := newService(t)
	p, _ := svc.Create(context.Background(), CreatePayload{ExternalID: "ext-1", FullName: "Alice"})
	if _, err := svc.AddAttribute(context.Background(), p.ID, AttributeCreatePayload{Key: "k", Value: "v"}); err != nil {
		t.Fatal(err)
	}
	sink.events = nil
	if err := svc.RemoveAttribute(context.Background(), p.ID, "k"); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.person.attribute_removed" {
		t.Fatalf("expected attribute_removed, got %+v", sink.events)
	}
	if err := svc.RemoveAttribute(context.Background(), p.ID, "k"); err == nil {
		t.Fatal("expected ErrAttributeNotFound on second remove")
	}
}

func TestBulkUpsert_emitsBulkEvent_andRowCount(t *testing.T) {
	svc, _, sink := newService(t)
	in := BulkPayload{Items: []BulkItem{
		{ExternalID: "ext-1", FullName: "A", Attributes: map[string]string{"k": "v"}},
		{ExternalID: "ext-2", FullName: "B"},
	}}
	res, err := svc.BulkUpsert(context.Background(), in)
	if err != nil {
		t.Fatalf("BulkUpsert: %v", err)
	}
	if res.RowCount != 2 {
		t.Fatalf("expected 2 rows, got %d", res.RowCount)
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.person.bulk_upserted" {
		t.Fatalf("expected bulk_upserted, got %+v", sink.events)
	}
	if v := sink.events[0].Payload["row_count"]; v != 2 {
		t.Fatalf("unexpected payload row_count: %v", v)
	}
}

func TestBulkUpsert_rejectsOversize(t *testing.T) {
	svc, _, _ := newService(t)
	items := make([]BulkItem, BulkLimit+1)
	for i := range items {
		items[i] = BulkItem{ExternalID: "x", FullName: "y"}
	}
	if _, err := svc.BulkUpsert(context.Background(), BulkPayload{Items: items}); !errors.Is(err, ErrBulkTooLarge) {
		t.Fatalf("expected ErrBulkTooLarge, got %v", err)
	}
}

func TestBulkUpsert_rejectsEmpty(t *testing.T) {
	svc, _, _ := newService(t)
	if _, err := svc.BulkUpsert(context.Background(), BulkPayload{Items: nil}); !errors.Is(err, ErrBulkEmpty) {
		t.Fatalf("expected ErrBulkEmpty, got %v", err)
	}
}
