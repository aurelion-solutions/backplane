// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package pipelines

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
	rows map[uuid.UUID]*Pipeline
}

func newMemRepo() *memRepo {
	return &memRepo{rows: map[uuid.UUID]*Pipeline{}}
}

func (r *memRepo) GetByID(_ context.Context, id uuid.UUID) (*Pipeline, error) {
	if p, ok := r.rows[id]; ok {
		return p, nil
	}
	return nil, ErrNotFound
}
func (r *memRepo) GetByNaturalKey(_ context.Context, cartridgeRef, name string) (*Pipeline, error) {
	for _, p := range r.rows {
		if p.CartridgeRef == cartridgeRef && p.Name == name {
			return p, nil
		}
	}
	return nil, ErrNotFound
}
func (r *memRepo) List(_ context.Context, f ListFilter) ([]*Pipeline, int, error) {
	out := []*Pipeline{}
	for _, p := range r.rows {
		if f.CartridgeRef != "" && p.CartridgeRef != f.CartridgeRef {
			continue
		}
		if !f.IncludeInactive && !p.IsActive {
			continue
		}
		out = append(out, p)
	}
	total := len(out)
	if f.Offset >= total {
		return []*Pipeline{}, total, nil
	}
	out = out[f.Offset:]
	if f.Limit > 0 && f.Limit < len(out) {
		out = out[:f.Limit]
	}
	return out, total, nil
}
func (r *memRepo) ListActiveByCartridge(_ context.Context, cartridgeRef string) ([]*Pipeline, error) {
	out := []*Pipeline{}
	for _, p := range r.rows {
		if p.CartridgeRef == cartridgeRef && p.IsActive {
			out = append(out, p)
		}
	}
	return out, nil
}
func (r *memRepo) Upsert(_ context.Context, p *Pipeline) error {
	for id, ex := range r.rows {
		if ex.CartridgeRef == p.CartridgeRef && ex.Name == p.Name {
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

func mustRow(t *testing.T, repo Repository, cartridge, name string, active bool) *Pipeline {
	t.Helper()
	now := time.Now().UTC()
	p := &Pipeline{
		ID:           uuid.New(),
		CartridgeRef: cartridge,
		Name:         name,
		Version:      1,
		ContentHash:  "deadbeef",
		SourcePath:   "/tmp/" + name + ".yaml",
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
	mustRow(t, repo, "popular", "p1", true)
	mustRow(t, repo, "popular", "p2", true)
	mustRow(t, repo, "glyph", "p3", true)

	e := echo.New()
	RegisterRoutes(e.Group("/v0"), repo)

	req := httptest.NewRequest(http.MethodGet, "/v0/pipelines?cartridge_ref=popular", nil)
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
}

func TestList_HidesInactiveByDefault(t *testing.T) {
	repo := newMemRepo()
	mustRow(t, repo, "popular", "alive", true)
	mustRow(t, repo, "popular", "dead", false)

	e := echo.New()
	RegisterRoutes(e.Group("/v0"), repo)

	req := httptest.NewRequest(http.MethodGet, "/v0/pipelines", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	var resp ListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 1 || resp.Items[0].Name != "alive" {
		t.Fatalf("got %+v", resp)
	}
}

func TestGet_NotFound(t *testing.T) {
	repo := newMemRepo()
	e := echo.New()
	RegisterRoutes(e.Group("/v0"), repo)

	req := httptest.NewRequest(http.MethodGet, "/v0/pipelines/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rec.Code)
	}
}

func TestRepository_MarkRemovedAndResurrect(t *testing.T) {
	repo := newMemRepo()
	p := mustRow(t, repo, "popular", "p1", true)
	ctx := context.Background()
	if err := repo.MarkRemoved(ctx, p.ID, time.Now().UTC()); err != nil {
		t.Fatalf("mark: %v", err)
	}
	got, _ := repo.GetByID(ctx, p.ID)
	if got.IsActive || got.RemovedAt == nil {
		t.Fatalf("expected inactive with removed_at")
	}
	if err := repo.Resurrect(ctx, p.ID, time.Now().UTC()); err != nil {
		t.Fatalf("resurrect: %v", err)
	}
	got, _ = repo.GetByID(ctx, p.ID)
	if !got.IsActive || got.RemovedAt != nil {
		t.Fatalf("expected active with nil removed_at")
	}
}
