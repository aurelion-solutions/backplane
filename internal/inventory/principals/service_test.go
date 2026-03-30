// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package principals

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
	rows map[uuid.UUID]*Principal
}

func newMemRepo() *memRepo { return &memRepo{rows: map[uuid.UUID]*Principal{}} }

func (r *memRepo) GetByID(_ context.Context, id uuid.UUID) (*Principal, error) {
	if p, ok := r.rows[id]; ok {
		return p, nil
	}
	return nil, ErrNotFound
}
func (r *memRepo) GetByBody(_ context.Context, kind shared.PrincipalKind, bid uuid.UUID) (*Principal, error) {
	for _, p := range r.rows {
		if p.Kind != kind {
			continue
		}
		switch kind {
		case shared.PrincipalKindEmployment:
			if p.PrincipalEmploymentID != nil && *p.PrincipalEmploymentID == bid {
				return p, nil
			}
		case shared.PrincipalKindWorkload:
			if p.PrincipalWorkloadID != nil && *p.PrincipalWorkloadID == bid {
				return p, nil
			}
		case shared.PrincipalKindCustomer:
			if p.PrincipalCustomerID != nil && *p.PrincipalCustomerID == bid {
				return p, nil
			}
		}
	}
	return nil, ErrNotFound
}
func (r *memRepo) List(_ context.Context, limit, offset int) ([]*Principal, int, error) {
	out := []*Principal{}
	for _, p := range r.rows {
		out = append(out, p)
	}
	total := len(out)
	if offset >= total {
		return []*Principal{}, total, nil
	}
	out = out[offset:]
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, total, nil
}
func (r *memRepo) Insert(_ context.Context, p *Principal) error {
	for _, ex := range r.rows {
		if ex.Kind == p.Kind && ex.ExternalID == p.ExternalID {
			return ErrDuplicate
		}
	}
	for _, ex := range r.rows {
		switch p.Kind {
		case shared.PrincipalKindEmployment:
			if p.PrincipalEmploymentID != nil && ex.PrincipalEmploymentID != nil && *ex.PrincipalEmploymentID == *p.PrincipalEmploymentID {
				return ErrBodyAlreadyBound
			}
		case shared.PrincipalKindWorkload:
			if p.PrincipalWorkloadID != nil && ex.PrincipalWorkloadID != nil && *ex.PrincipalWorkloadID == *p.PrincipalWorkloadID {
				return ErrBodyAlreadyBound
			}
		case shared.PrincipalKindCustomer:
			if p.PrincipalCustomerID != nil && ex.PrincipalCustomerID != nil && *ex.PrincipalCustomerID == *p.PrincipalCustomerID {
				return ErrBodyAlreadyBound
			}
		}
	}
	r.rows[p.ID] = p
	return nil
}
func (r *memRepo) UpdateStatus(_ context.Context, id uuid.UUID, status string, updatedAt time.Time) error {
	if p, ok := r.rows[id]; ok {
		p.Status = status
		p.UpdatedAt = updatedAt
	}
	return nil
}
func (r *memRepo) UpdateLock(_ context.Context, id uuid.UUID, isLocked bool, updatedAt time.Time) error {
	if p, ok := r.rows[id]; ok {
		p.IsLocked = isLocked
		p.UpdatedAt = updatedAt
	}
	return nil
}
func (r *memRepo) ListAttributes(_ context.Context, _ uuid.UUID) ([]*PrincipalAttribute, error) {
	return nil, nil
}

type recordingSink struct{ events []events.Envelope }

func (s *recordingSink) Emit(_ context.Context, env events.Envelope) error {
	s.events = append(s.events, env)
	return nil
}

type stubEmployments struct {
	codes map[uuid.UUID]string // id -> code; absence = not exists
}

func (s *stubEmployments) EmploymentCode(_ context.Context, id uuid.UUID) (string, bool, error) {
	code, ok := s.codes[id]
	return code, ok, nil
}

type stubWorkloads struct {
	exists map[uuid.UUID]bool
}

func (s *stubWorkloads) WorkloadExists(_ context.Context, id uuid.UUID) (bool, error) {
	return s.exists[id], nil
}

type stubCustomers struct {
	rows map[uuid.UUID]CustomerStateView
}

func (s *stubCustomers) CustomerState(_ context.Context, id uuid.UUID) (CustomerStateView, error) {
	v, ok := s.rows[id]
	if !ok {
		return CustomerStateView{Exists: false}, nil
	}
	v.Exists = true
	return v, nil
}

func newService(t *testing.T) (*Service, *memRepo, *recordingSink, *stubEmployments, *stubWorkloads, *stubCustomers) {
	t.Helper()
	repo := newMemRepo()
	sink := &recordingSink{}
	emp := &stubEmployments{codes: map[uuid.UUID]string{}}
	wls := &stubWorkloads{exists: map[uuid.UUID]bool{}}
	cust := &stubCustomers{rows: map[uuid.UUID]CustomerStateView{}}
	svc := NewService(Deps{
		Repo: repo, Sink: sink,
		Sources: BodySources{Employments: emp, Workloads: wls, Customers: cust},
		Now:     func() time.Time { return time.Unix(1700000000, 0).UTC() },
	})
	return svc, repo, sink, emp, wls, cust
}

func TestCreate_employmentStatus_mirrorsCode(t *testing.T) {
	svc, _, sink, emp, _, _ := newService(t)
	eid := uuid.New()
	emp.codes[eid] = "probation"
	row, err := svc.Create(context.Background(), CreatePayload{
		ExternalID: "emp-1", Kind: shared.PrincipalKindEmployment, PrincipalEmploymentID: &eid,
	})
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != "probation" {
		t.Fatalf("expected status=probation, got %q", row.Status)
	}
	if row.IsLocked {
		t.Fatal("freshly created principal must not be locked")
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.principal.created" {
		t.Fatalf("expected created, got %+v", sink.events)
	}
}

func TestCreate_rejectsTwoBodies(t *testing.T) {
	svc, _, _, _, _, _ := newService(t)
	a := uuid.New()
	b := uuid.New()
	_, err := svc.Create(context.Background(), CreatePayload{
		ExternalID: "x", Kind: shared.PrincipalKindEmployment,
		PrincipalEmploymentID: &a, PrincipalWorkloadID: &b,
	})
	if err == nil {
		t.Fatal("expected validation error on two body ids")
	}
}

func TestCreate_kindMismatch(t *testing.T) {
	svc, _, _, _, _, _ := newService(t)
	wid := uuid.New()
	_, err := svc.Create(context.Background(), CreatePayload{
		ExternalID: "x", Kind: shared.PrincipalKindEmployment, PrincipalWorkloadID: &wid,
	})
	if err == nil {
		t.Fatal("expected validation error on kind/body mismatch")
	}
}

func TestCreate_unknownBody(t *testing.T) {
	svc, _, _, _, _, _ := newService(t)
	pid := uuid.New()
	_, err := svc.Create(context.Background(), CreatePayload{
		ExternalID: "x", Kind: shared.PrincipalKindCustomer, PrincipalCustomerID: &pid,
	})
	if !errors.Is(err, ErrBodyNotFound) {
		t.Fatalf("expected ErrBodyNotFound, got %v", err)
	}
}

func TestCreate_bodyAlreadyBound(t *testing.T) {
	svc, _, _, emp, _, _ := newService(t)
	eid := uuid.New()
	emp.codes[eid] = "active"
	if _, err := svc.Create(context.Background(), CreatePayload{
		ExternalID: "a", Kind: shared.PrincipalKindEmployment, PrincipalEmploymentID: &eid,
	}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Create(context.Background(), CreatePayload{
		ExternalID: "b", Kind: shared.PrincipalKindEmployment, PrincipalEmploymentID: &eid,
	})
	if !errors.Is(err, ErrBodyAlreadyBound) {
		t.Fatalf("expected ErrBodyAlreadyBound, got %v", err)
	}
}

func TestCreate_customerStatus_derivation(t *testing.T) {
	svc, _, _, _, _, cust := newService(t)
	cid := uuid.New()
	cust.rows[cid] = CustomerStateView{EmailVerified: false}
	row, err := svc.Create(context.Background(), CreatePayload{
		ExternalID: "c-1", Kind: shared.PrincipalKindCustomer, PrincipalCustomerID: &cid,
	})
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != string(shared.CustomerStatusRegistered) {
		t.Fatalf("expected registered, got %q", row.Status)
	}
}

func TestRecompute_codeChange_emitsRecomputed(t *testing.T) {
	svc, _, sink, emp, _, _ := newService(t)
	eid := uuid.New()
	emp.codes[eid] = "probation"
	row, _ := svc.Create(context.Background(), CreatePayload{
		ExternalID: "e-1", Kind: shared.PrincipalKindEmployment, PrincipalEmploymentID: &eid,
	})
	sink.events = nil

	emp.codes[eid] = "active"
	if err := svc.RecomputeForBody(context.Background(), shared.PrincipalKindEmployment, eid); err != nil {
		t.Fatal(err)
	}
	if row.Status != "active" {
		t.Fatalf("expected status updated in-place to active, got %q", row.Status)
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.principal.status_recomputed" {
		t.Fatalf("expected status_recomputed, got %+v", sink.events)
	}
}

func TestRecompute_noChange_noEvent(t *testing.T) {
	svc, _, sink, _, _, cust := newService(t)
	cid := uuid.New()
	cust.rows[cid] = CustomerStateView{EmailVerified: true}
	if _, err := svc.Create(context.Background(), CreatePayload{
		ExternalID: "c-1", Kind: shared.PrincipalKindCustomer, PrincipalCustomerID: &cid,
	}); err != nil {
		t.Fatal(err)
	}
	sink.events = nil
	if err := svc.RecomputeForBody(context.Background(), shared.PrincipalKindCustomer, cid); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 0 {
		t.Fatalf("expected no event on noop, got %+v", sink.events)
	}
}

func TestRecompute_missingPrincipal_returnsTypedError(t *testing.T) {
	svc, _, _, _, _, _ := newService(t)
	if err := svc.RecomputeForBody(context.Background(), shared.PrincipalKindCustomer, uuid.New()); !errors.Is(err, ErrPrincipalMissingForBody) {
		t.Fatalf("expected ErrPrincipalMissingForBody, got %v", err)
	}
}

func TestWorkloadStatusDerivation_existsActive(t *testing.T) {
	svc, _, _, _, wls, _ := newService(t)
	wid := uuid.New()
	wls.exists[wid] = true
	row, err := svc.Create(context.Background(), CreatePayload{
		ExternalID: "w-1", Kind: shared.PrincipalKindWorkload, PrincipalWorkloadID: &wid,
	})
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != string(shared.WorkloadStatusActive) {
		t.Fatalf("expected workload status=active, got %q", row.Status)
	}
}

func TestLock_setsFlagEmitsEvent_idempotent(t *testing.T) {
	svc, _, sink, _, wls, _ := newService(t)
	wid := uuid.New()
	wls.exists[wid] = true
	row, _ := svc.Create(context.Background(), CreatePayload{
		ExternalID: "w-1", Kind: shared.PrincipalKindWorkload, PrincipalWorkloadID: &wid,
	})
	sink.events = nil

	reason := "admin paused"
	locked, err := svc.Lock(context.Background(), row.ID, LockPayload{Reason: &reason})
	if err != nil {
		t.Fatal(err)
	}
	if !locked.IsLocked {
		t.Fatal("expected is_locked=true")
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.principal.locked" {
		t.Fatalf("expected locked event, got %+v", sink.events)
	}
	if sink.events[0].Payload["reason"] != "admin paused" {
		t.Fatalf("expected reason in payload, got %+v", sink.events[0].Payload)
	}

	sink.events = nil
	if _, err := svc.Lock(context.Background(), row.ID, LockPayload{}); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 0 {
		t.Fatalf("expected no event on already-locked, got %+v", sink.events)
	}
}

func TestUnlock_setsFlagEmitsEvent_idempotent(t *testing.T) {
	svc, _, sink, _, wls, _ := newService(t)
	wid := uuid.New()
	wls.exists[wid] = true
	row, _ := svc.Create(context.Background(), CreatePayload{
		ExternalID: "w-1", Kind: shared.PrincipalKindWorkload, PrincipalWorkloadID: &wid,
	})
	if _, err := svc.Lock(context.Background(), row.ID, LockPayload{}); err != nil {
		t.Fatal(err)
	}
	sink.events = nil
	if _, err := svc.Unlock(context.Background(), row.ID); err != nil {
		t.Fatal(err)
	}
	if row.IsLocked {
		t.Fatal("expected is_locked=false after unlock")
	}
	if len(sink.events) != 1 || sink.events[0].EventType != "inventory.principal.unlocked" {
		t.Fatalf("expected unlocked event, got %+v", sink.events)
	}
	sink.events = nil
	if _, err := svc.Unlock(context.Background(), row.ID); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) != 0 {
		t.Fatalf("expected no event on already-unlocked, got %+v", sink.events)
	}
}
