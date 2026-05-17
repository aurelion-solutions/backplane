// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policies

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type memRepo struct {
	rows map[uuid.UUID]*Policy
}

func newMemRepo() *memRepo {
	return &memRepo{rows: map[uuid.UUID]*Policy{}}
}

func (r *memRepo) GetByID(_ context.Context, id uuid.UUID) (*Policy, error) {
	if p, ok := r.rows[id]; ok {
		return p, nil
	}
	return nil, ErrNotFound
}
func (r *memRepo) GetByNaturalKey(_ context.Context, cartridgeRef, ruleID string) (*Policy, error) {
	for _, p := range r.rows {
		if p.CartridgeRef == cartridgeRef && p.RuleID == ruleID {
			return p, nil
		}
	}
	return nil, ErrNotFound
}
func (r *memRepo) List(_ context.Context, f ListFilter) ([]*Policy, int, error) {
	out := []*Policy{}
	for _, p := range r.rows {
		if f.CartridgeRef != "" && p.CartridgeRef != f.CartridgeRef {
			continue
		}
		if f.Mechanism != "" && p.Mechanism != f.Mechanism {
			continue
		}
		if !f.IncludeInactive && !p.IsActive {
			continue
		}
		out = append(out, p)
	}
	total := len(out)
	if f.Offset >= total {
		return []*Policy{}, total, nil
	}
	out = out[f.Offset:]
	if f.Limit > 0 && f.Limit < len(out) {
		out = out[:f.Limit]
	}
	return out, total, nil
}
func (r *memRepo) ListActiveByCartridge(_ context.Context, cartridgeRef string) ([]*Policy, error) {
	out := []*Policy{}
	for _, p := range r.rows {
		if p.CartridgeRef == cartridgeRef && p.IsActive {
			out = append(out, p)
		}
	}
	return out, nil
}
func (r *memRepo) ListActiveByMechanisms(_ context.Context, ms []string) ([]*Policy, error) {
	set := map[string]struct{}{}
	for _, m := range ms {
		set[m] = struct{}{}
	}
	out := []*Policy{}
	for _, p := range r.rows {
		if _, ok := set[p.Mechanism]; !ok {
			continue
		}
		if !p.IsActive {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}
func (r *memRepo) Upsert(_ context.Context, p *Policy) error {
	for id, ex := range r.rows {
		if ex.CartridgeRef == p.CartridgeRef && ex.RuleID == p.RuleID {
			p.ID = id
			r.rows[id] = p
			return nil
		}
	}
	r.rows[p.ID] = p
	return nil
}
func (r *memRepo) MarkRemoved(_ context.Context, id uuid.UUID, removedAt time.Time) error {
	p, ok := r.rows[id]
	if !ok {
		return ErrNotFound
	}
	if !p.IsActive {
		return ErrNotFound
	}
	p.IsActive = false
	p.RemovedAt = &removedAt
	p.UpdatedAt = removedAt
	return nil
}
func (r *memRepo) Resurrect(_ context.Context, id uuid.UUID, now time.Time) error {
	p, ok := r.rows[id]
	if !ok {
		return ErrNotFound
	}
	p.IsActive = true
	p.RemovedAt = nil
	p.UpdatedAt = now
	return nil
}

func mustRow(t *testing.T, repo Repository, cartridge, ruleID, mech string, active bool) *Policy {
	t.Helper()
	now := time.Now().UTC()
	p := &Policy{
		ID:           uuid.New(),
		CartridgeRef: cartridge,
		RuleID:       ruleID,
		Name:         ruleID,
		Mechanism:    mech,
		Version:      1,
		IsActive:     active,
		Meta:         map[string]any{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if !active {
		ts := now
		p.RemovedAt = &ts
	}
	if err := repo.Upsert(context.Background(), p); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	return p
}

func TestList_FiltersByCartridge(t *testing.T) {
	repo := newMemRepo()
	mustRow(t, repo, "popular", "r1", "generic", true)
	mustRow(t, repo, "popular", "r2", "sod", true)
	mustRow(t, repo, "glyph", "r3", "generic", true)

	e := echo.New()
	RegisterRoutes(e.Group("/v0"), repo)

	req := httptest.NewRequest(http.MethodGet, "/v0/policies?cartridge_ref=popular", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp ListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 2 {
		t.Fatalf("total=%d want 2", resp.Total)
	}
	for _, p := range resp.Items {
		if p.CartridgeRef != "popular" {
			t.Fatalf("got %s want popular", p.CartridgeRef)
		}
	}
}

func TestList_HidesInactiveByDefault(t *testing.T) {
	repo := newMemRepo()
	mustRow(t, repo, "popular", "alive", "generic", true)
	mustRow(t, repo, "popular", "dead", "generic", false)

	e := echo.New()
	RegisterRoutes(e.Group("/v0"), repo)

	req := httptest.NewRequest(http.MethodGet, "/v0/policies", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var resp ListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 1 {
		t.Fatalf("total=%d want 1 (inactive should be hidden)", resp.Total)
	}
	if resp.Items[0].RuleID != "alive" {
		t.Fatalf("got %s want alive", resp.Items[0].RuleID)
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/policies?include_inactive=true", nil)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 2 {
		t.Fatalf("total=%d want 2 (include_inactive=true)", resp.Total)
	}
}

func TestGet_ReturnsNotFound(t *testing.T) {
	repo := newMemRepo()
	e := echo.New()
	RegisterRoutes(e.Group("/v0"), repo)

	req := httptest.NewRequest(http.MethodGet, "/v0/policies/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rec.Code)
	}
}

func TestGet_ReturnsRow(t *testing.T) {
	repo := newMemRepo()
	p := mustRow(t, repo, "popular", "r1", "generic", true)

	e := echo.New()
	RegisterRoutes(e.Group("/v0"), repo)

	req := httptest.NewRequest(http.MethodGet, "/v0/policies/"+p.ID.String(), nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got Policy
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.RuleID != "r1" {
		t.Fatalf("got rule_id=%s", got.RuleID)
	}
}

func TestRepository_MarkRemoved_AndResurrect(t *testing.T) {
	repo := newMemRepo()
	p := mustRow(t, repo, "popular", "r1", "generic", true)
	ctx := context.Background()

	if err := repo.MarkRemoved(ctx, p.ID, time.Now().UTC()); err != nil {
		t.Fatalf("mark removed: %v", err)
	}
	got, err := repo.GetByID(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.IsActive {
		t.Fatalf("expected inactive after MarkRemoved")
	}
	if got.RemovedAt == nil {
		t.Fatalf("expected removed_at set")
	}

	if err := repo.Resurrect(ctx, p.ID, time.Now().UTC()); err != nil {
		t.Fatalf("resurrect: %v", err)
	}
	got, _ = repo.GetByID(ctx, p.ID)
	if !got.IsActive {
		t.Fatalf("expected active after Resurrect")
	}
	if got.RemovedAt != nil {
		t.Fatalf("expected removed_at cleared")
	}
}
