// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package customers

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
	rows  map[uuid.UUID]*Customer
	attrs map[uuid.UUID]map[string]*CustomerAttribute
}

func newMemRepo() *memRepo {
	return &memRepo{
		rows:  map[uuid.UUID]*Customer{},
		attrs: map[uuid.UUID]map[string]*CustomerAttribute{},
	}
}

func (r *memRepo) GetByID(_ context.Context, id uuid.UUID) (*Customer, error) {
	if c, ok := r.rows[id]; ok {
		return c, nil
	}
	return nil, ErrNotFound
}
func (r *memRepo) GetByExternalID(_ context.Context, ext string) (*Customer, error) {
	for _, c := range r.rows {
		if c.ExternalID == ext {
			return c, nil
		}
	}
	return nil, ErrNotFound
}
func (r *memRepo) List(_ context.Context, limit, offset int) ([]*Customer, int, error) {
	out := []*Customer{}
	for _, c := range r.rows {
		out = append(out, c)
	}
	total := len(out)
	if offset >= total {
		return []*Customer{}, total, nil
	}
	out = out[offset:]
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, total, nil
}
func (r *memRepo) Insert(_ context.Context, c *Customer) error {
	for _, ex := range r.rows {
		if ex.ExternalID == c.ExternalID {
			return ErrExternalIDAlreadyExists
		}
	}
	r.rows[c.ID] = c
	return nil
}
func (r *memRepo) Update(_ context.Context, c *Customer) error {
	r.rows[c.ID] = c
	return nil
}
func (r *memRepo) ListAttributes(_ context.Context, id uuid.UUID) ([]*CustomerAttribute, error) {
	out := []*CustomerAttribute{}
	for _, a := range r.attrs[id] {
		out = append(out, a)
	}
	return out, nil
}
func (r *memRepo) GetAttribute(_ context.Context, id uuid.UUID, key string) (*CustomerAttribute, error) {
	if a, ok := r.attrs[id][key]; ok {
		return a, nil
	}
	return nil, ErrAttributeNotFound
}
func (r *memRepo) UpsertAttribute(_ context.Context, a *CustomerAttribute) error {
	if r.attrs[a.CustomerID] == nil {
		r.attrs[a.CustomerID] = map[string]*CustomerAttribute{}
	}
	r.attrs[a.CustomerID][a.Key] = a
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
		var existing *Customer
		for _, c := range r.rows {
			if c.ExternalID == it.ExternalID {
				existing = c
				break
			}
		}
		if existing == nil {
			existing = &Customer{ID: idGen(), ExternalID: it.ExternalID}
			r.rows[existing.ID] = existing
		}
		if it.EmailVerified != nil {
			existing.EmailVerified = *it.EmailVerified
		}
		if it.PlanTier != nil {
			pt := *it.PlanTier
			existing.PlanTier = &pt
		}
	}
	return len(items), nil
}

type recordingSink struct{ events []events.Envelope }

func (s *recordingSink) Emit(_ context.Context, env events.Envelope) error {
	s.events = append(s.events, env)
	return nil
}

type stubRecomputer struct {
	calls []uuid.UUID
}

func (s *stubRecomputer) RecomputeForBody(_ context.Context, kind shared.PrincipalKind, id uuid.UUID) error {
	if kind != shared.PrincipalKindCustomer {
		return errors.New("expected customer kind")
	}
	s.calls = append(s.calls, id)
	return nil
}

func newService(t *testing.T) (*Service, *memRepo, *recordingSink, *stubRecomputer) {
	t.Helper()
	repo := newMemRepo()
	sink := &recordingSink{}
	rec := &stubRecomputer{}
	now := func() time.Time { return time.Unix(1700000000, 0).UTC() }
	svc := NewService(Deps{Repo: repo, Sink: sink, Recomputer: rec, Now: now})
	return svc, repo, sink, rec
}

func TestCreate_emitsEventWithTenantAndPlan(t *testing.T) {
	svc, _, sink, _ := newService(t)
	tid := "tenant-x"
	pt := shared.CustomerPlanTierPro
	_, err := svc.Create(context.Background(), CreatePayload{
		ExternalID: "cust-1", TenantID: &tid, PlanTier: &pt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.customer.created" {
		t.Fatalf("expected created event, got %+v", sink.events)
	}
	if sink.events[0].Payload["tenant_id"] != "tenant-x" || sink.events[0].Payload["plan_tier"] != "pro" {
		t.Fatalf("missing tenant/plan in payload: %+v", sink.events[0].Payload)
	}
}

func TestCreate_rejectsInvalidPlanTier(t *testing.T) {
	svc, _, _, _ := newService(t)
	bad := shared.CustomerPlanTier("premium")
	_, err := svc.Create(context.Background(), CreatePayload{ExternalID: "cust-1", PlanTier: &bad})
	if !errors.Is(err, ErrInvalidEnum) {
		t.Fatalf("expected ErrInvalidEnum, got %v", err)
	}
}

func TestCreate_duplicateExternalID(t *testing.T) {
	svc, _, _, _ := newService(t)
	_, _ = svc.Create(context.Background(), CreatePayload{ExternalID: "dup"})
	_, err := svc.Create(context.Background(), CreatePayload{ExternalID: "dup"})
	if !errors.Is(err, ErrExternalIDAlreadyExists) {
		t.Fatalf("expected ErrExternalIDAlreadyExists, got %v", err)
	}
}

func TestUpdate_strictFourField_listsChanged(t *testing.T) {
	svc, _, sink, rec := newService(t)
	c, _ := svc.Create(context.Background(), CreatePayload{ExternalID: "cust-1"})
	sink.events = nil
	rec.calls = nil
	yes := true
	pt := shared.CustomerPlanTierEnterprise
	_, err := svc.Update(context.Background(), c.ID, PatchPayload{EmailVerified: &yes, PlanTier: &pt})
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.customer.updated" {
		t.Fatalf("expected updated, got %+v", sink.events)
	}
	changed, ok := sink.events[0].Payload["changed_fields"].([]string)
	if !ok {
		t.Fatalf("changed_fields missing: %+v", sink.events[0].Payload)
	}
	// sorted alphabetical
	if len(changed) != 2 || changed[0] != "email_verified" || changed[1] != "plan_tier" {
		t.Fatalf("unexpected changed_fields: %v", changed)
	}
	// email_verified change → recompute
	if len(rec.calls) != 1 || rec.calls[0] != c.ID {
		t.Fatalf("expected recompute call, got %+v", rec.calls)
	}
}


func TestUpdate_planTierOnly_noRecompute(t *testing.T) {
	svc, _, _, rec := newService(t)
	c, _ := svc.Create(context.Background(), CreatePayload{ExternalID: "cust-1"})
	rec.calls = nil
	pt := shared.CustomerPlanTierBasic
	if _, err := svc.Update(context.Background(), c.ID, PatchPayload{PlanTier: &pt}); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 0 {
		t.Fatalf("expected no recompute call, got %+v", rec.calls)
	}
}

func TestBulkUpsert_emitsBulkEvent(t *testing.T) {
	svc, _, sink, _ := newService(t)
	res, err := svc.BulkUpsert(context.Background(), BulkPayload{Items: []BulkItem{
		{ExternalID: "cust-1"},
		{ExternalID: "cust-2"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if res.RowCount != 2 {
		t.Fatalf("expected 2, got %d", res.RowCount)
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.customer.bulk_upserted" {
		t.Fatalf("expected bulk_upserted, got %+v", sink.events)
	}
}
