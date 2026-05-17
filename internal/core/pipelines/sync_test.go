// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package pipelines

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/loader"
	"github.com/aurelion-solutions/backplane/internal/inventory/pipelines"
	"github.com/google/uuid"
)

type stubProvider struct {
	refs  []cartridges.Ref
	pipes map[string][]string
}

func (s *stubProvider) List() ([]cartridges.Ref, error)              { return s.refs, nil }
func (s *stubProvider) Materialize(_ cartridges.Ref) (string, error) { return "", nil }
func (s *stubProvider) Policies(_ cartridges.Ref) (map[string]cartridges.Manifest, error) {
	return nil, nil
}
func (s *stubProvider) Pipelines(ref cartridges.Ref) ([]string, error) {
	return s.pipes[ref.ID], nil
}
func (s *stubProvider) Apps(_ cartridges.Ref) (map[string]cartridges.AppCartridge, error) {
	return nil, nil
}

type stubRepo struct {
	rows map[uuid.UUID]*pipelines.Pipeline
}

func newStubRepo() *stubRepo { return &stubRepo{rows: map[uuid.UUID]*pipelines.Pipeline{}} }

func (r *stubRepo) GetByID(_ context.Context, id uuid.UUID) (*pipelines.Pipeline, error) {
	if p, ok := r.rows[id]; ok {
		return p, nil
	}
	return nil, pipelines.ErrNotFound
}
func (r *stubRepo) GetByNaturalKey(_ context.Context, cart, name string) (*pipelines.Pipeline, error) {
	for _, p := range r.rows {
		if p.CartridgeRef == cart && p.Name == name {
			return p, nil
		}
	}
	return nil, pipelines.ErrNotFound
}
func (r *stubRepo) List(_ context.Context, _ pipelines.ListFilter) ([]*pipelines.Pipeline, int, error) {
	return nil, 0, nil
}
func (r *stubRepo) ListActiveByCartridge(_ context.Context, cart string) ([]*pipelines.Pipeline, error) {
	out := []*pipelines.Pipeline{}
	for _, p := range r.rows {
		if p.CartridgeRef == cart && p.IsActive {
			out = append(out, p)
		}
	}
	return out, nil
}
func (r *stubRepo) Upsert(_ context.Context, p *pipelines.Pipeline) error {
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
func (r *stubRepo) MarkRemoved(_ context.Context, id uuid.UUID, ts time.Time) error {
	p, ok := r.rows[id]
	if !ok || !p.IsActive {
		return pipelines.ErrNotFound
	}
	p.IsActive = false
	p.RemovedAt = &ts
	p.UpdatedAt = ts
	return nil
}
func (r *stubRepo) Resurrect(_ context.Context, id uuid.UUID, ts time.Time) error {
	p, ok := r.rows[id]
	if !ok {
		return pipelines.ErrNotFound
	}
	p.IsActive = true
	p.RemovedAt = nil
	p.UpdatedAt = ts
	return nil
}

const samplePipeline = `pipeline:
  schema_version: 1
  name: %s
  version: 1
  triggers: []
  steps:
    - name: only
      engine: echo
      action: echo
      args:
        msg: hi
`

func writePipeline(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, name+".yaml")
	body := []byte(`pipeline:
  schema_version: 1
  name: ` + name + `
  version: 1
  triggers: []
  steps:
    - name: only
      engine: echo
      action: echo
      args:
        msg: hi
`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func newSync(t *testing.T, prov cartridges.Provider, repo pipelines.Repository) *Manager {
	t.Helper()
	return New(Deps{
		Provider: prov,
		Loader:   loader.New(),
		Repo:     repo,
		IDGen:    uuid.New,
		Now:      func() time.Time { return time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC) },
	})
}

func TestSync_InsertAndRemove(t *testing.T) {
	dir := t.TempDir()
	p1 := writePipeline(t, dir, "p1")

	prov := &stubProvider{
		refs:  []cartridges.Ref{{ID: "popular"}},
		pipes: map[string][]string{"popular": {p1}},
	}
	repo := newStubRepo()
	m := newSync(t, prov, repo)

	r, err := m.Sync(context.Background())
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if r.Inserted != 1 {
		t.Fatalf("inserted=%d", r.Inserted)
	}
	got, _ := repo.GetByNaturalKey(context.Background(), "popular", "p1")
	if got.ContentHash == "" {
		t.Fatalf("content hash empty")
	}
	firstHash := got.ContentHash

	// No changes - second sync is idempotent.
	r, _ = m.Sync(context.Background())
	if r.Updated != 1 {
		// Upsert always runs (cheap), every existing-row counts as updated.
		t.Fatalf("second sync updated=%d want 1 (idempotent upsert)", r.Updated)
	}

	got2, _ := repo.GetByNaturalKey(context.Background(), "popular", "p1")
	if got2.ContentHash != firstHash {
		t.Fatalf("hash changed across identical sync")
	}

	// Remove the file - next sync soft-deletes.
	if err := os.Remove(p1); err != nil {
		t.Fatalf("remove: %v", err)
	}
	prov.pipes["popular"] = []string{}
	r, _ = m.Sync(context.Background())
	if r.Removed != 1 {
		t.Fatalf("removed=%d", r.Removed)
	}
	got3, _ := repo.GetByNaturalKey(context.Background(), "popular", "p1")
	if got3.IsActive {
		t.Fatalf("expected soft-deleted")
	}
}

func TestSync_BadYAMLSkipped(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("garbage:\n  : not valid"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	good := writePipeline(t, dir, "good")

	prov := &stubProvider{
		refs:  []cartridges.Ref{{ID: "c1"}},
		pipes: map[string][]string{"c1": {bad, good}},
	}
	repo := newStubRepo()
	m := newSync(t, prov, repo)

	r, err := m.Sync(context.Background())
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if r.Inserted != 1 {
		t.Fatalf("inserted=%d want 1 (good only)", r.Inserted)
	}
}
