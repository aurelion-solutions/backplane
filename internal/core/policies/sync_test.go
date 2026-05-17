// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policies

import (
	"context"
	"testing"
	"time"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
	"github.com/aurelion-solutions/backplane/internal/inventory/policies"
	"github.com/google/uuid"
)

type stubProvider struct {
	cartridges.Provider
	refs     []cartridges.Ref
	policies map[string]map[string]cartridges.Manifest
}

func (s *stubProvider) List() ([]cartridges.Ref, error) { return s.refs, nil }
func (s *stubProvider) Policies(ref cartridges.Ref) (map[string]cartridges.Manifest, error) {
	return s.policies[ref.ID], nil
}
func (s *stubProvider) Materialize(_ cartridges.Ref) (string, error) { return "", nil }
func (s *stubProvider) Pipelines(_ cartridges.Ref) ([]string, error) { return nil, nil }

type stubRepo struct {
	rows map[uuid.UUID]*policies.Policy
}

func newStubRepo() *stubRepo { return &stubRepo{rows: map[uuid.UUID]*policies.Policy{}} }

func (r *stubRepo) GetByID(_ context.Context, id uuid.UUID) (*policies.Policy, error) {
	if p, ok := r.rows[id]; ok {
		return p, nil
	}
	return nil, policies.ErrNotFound
}
func (r *stubRepo) GetByNaturalKey(_ context.Context, cart, rule string) (*policies.Policy, error) {
	for _, p := range r.rows {
		if p.CartridgeRef == cart && p.RuleID == rule {
			return p, nil
		}
	}
	return nil, policies.ErrNotFound
}
func (r *stubRepo) List(_ context.Context, _ policies.ListFilter) ([]*policies.Policy, int, error) {
	return nil, 0, nil
}
func (r *stubRepo) ListActiveByCartridge(_ context.Context, cart string) ([]*policies.Policy, error) {
	out := []*policies.Policy{}
	for _, p := range r.rows {
		if p.CartridgeRef == cart && p.IsActive {
			out = append(out, p)
		}
	}
	return out, nil
}
func (r *stubRepo) ListActiveByMechanisms(_ context.Context, _ []string) ([]*policies.Policy, error) {
	return nil, nil
}
func (r *stubRepo) Upsert(_ context.Context, p *policies.Policy) error {
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
func (r *stubRepo) MarkRemoved(_ context.Context, id uuid.UUID, ts time.Time) error {
	p, ok := r.rows[id]
	if !ok || !p.IsActive {
		return policies.ErrNotFound
	}
	p.IsActive = false
	p.RemovedAt = &ts
	p.UpdatedAt = ts
	return nil
}
func (r *stubRepo) Resurrect(_ context.Context, id uuid.UUID, ts time.Time) error {
	p, ok := r.rows[id]
	if !ok {
		return policies.ErrNotFound
	}
	p.IsActive = true
	p.RemovedAt = nil
	p.UpdatedAt = ts
	return nil
}

func newSync(t *testing.T, prov cartridges.Provider, repo policies.Repository) *Manager {
	t.Helper()
	return New(Deps{
		Provider: prov,
		Repo:     repo,
		IDGen:    uuid.New,
		Now:      func() time.Time { return time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC) },
	})
}

func TestSync_Insert(t *testing.T) {
	prov := &stubProvider{
		refs: []cartridges.Ref{{ID: "popular"}},
		policies: map[string]map[string]cartridges.Manifest{
			"popular": {
				"r1": {RuleID: "r1", Version: 1, Name: "rule one", Mechanism: "generic"},
			},
		},
	}
	repo := newStubRepo()
	m := newSync(t, prov, repo)
	r, err := m.Sync(context.Background())
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if r.Inserted != 1 || r.Updated != 0 || r.Removed != 0 {
		t.Fatalf("report=%+v", r)
	}
	got, err := repo.GetByNaturalKey(context.Background(), "popular", "r1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Mechanism != "generic" {
		t.Fatalf("mechanism=%s", got.Mechanism)
	}
	if !got.IsActive {
		t.Fatalf("expected active")
	}
}

func TestSync_UpdateBumpsVersion(t *testing.T) {
	prov := &stubProvider{
		refs: []cartridges.Ref{{ID: "popular"}},
		policies: map[string]map[string]cartridges.Manifest{
			"popular": {
				"r1": {RuleID: "r1", Version: 1, Name: "v1", Mechanism: "generic"},
			},
		},
	}
	repo := newStubRepo()
	m := newSync(t, prov, repo)
	if _, err := m.Sync(context.Background()); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	before, _ := repo.GetByNaturalKey(context.Background(), "popular", "r1")

	prov.policies["popular"]["r1"] = cartridges.Manifest{
		RuleID: "r1", Version: 2, Name: "v2", Mechanism: "generic",
	}
	r, err := m.Sync(context.Background())
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if r.Updated != 1 || r.Inserted != 0 {
		t.Fatalf("report=%+v", r)
	}
	after, _ := repo.GetByNaturalKey(context.Background(), "popular", "r1")
	if after.Version != 2 || after.Name != "v2" {
		t.Fatalf("after=%+v", after)
	}
	if after.ID != before.ID {
		t.Fatalf("id changed across updates")
	}
}

func TestSync_RemoveWhenManifestDisappears(t *testing.T) {
	prov := &stubProvider{
		refs: []cartridges.Ref{{ID: "popular"}},
		policies: map[string]map[string]cartridges.Manifest{
			"popular": {
				"r1": {RuleID: "r1", Version: 1, Name: "v1", Mechanism: "generic"},
			},
		},
	}
	repo := newStubRepo()
	m := newSync(t, prov, repo)
	if _, err := m.Sync(context.Background()); err != nil {
		t.Fatalf("sync: %v", err)
	}
	prov.policies["popular"] = map[string]cartridges.Manifest{}
	r, err := m.Sync(context.Background())
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if r.Removed != 1 {
		t.Fatalf("report=%+v", r)
	}
	got, _ := repo.GetByNaturalKey(context.Background(), "popular", "r1")
	if got.IsActive {
		t.Fatalf("expected inactive")
	}
}

func TestSync_ResurrectWhenManifestReturns(t *testing.T) {
	prov := &stubProvider{
		refs: []cartridges.Ref{{ID: "popular"}},
		policies: map[string]map[string]cartridges.Manifest{
			"popular": {
				"r1": {RuleID: "r1", Version: 1, Name: "v1", Mechanism: "generic"},
			},
		},
	}
	repo := newStubRepo()
	m := newSync(t, prov, repo)
	_, _ = m.Sync(context.Background())
	before, _ := repo.GetByNaturalKey(context.Background(), "popular", "r1")

	prov.policies["popular"] = map[string]cartridges.Manifest{}
	_, _ = m.Sync(context.Background())
	dead, _ := repo.GetByNaturalKey(context.Background(), "popular", "r1")
	if dead.IsActive {
		t.Fatalf("expected dead inactive")
	}

	prov.policies["popular"]["r1"] = cartridges.Manifest{
		RuleID: "r1", Version: 1, Name: "v1", Mechanism: "generic",
	}
	if _, err := m.Sync(context.Background()); err != nil {
		t.Fatalf("third sync: %v", err)
	}
	alive, _ := repo.GetByNaturalKey(context.Background(), "popular", "r1")
	if !alive.IsActive {
		t.Fatalf("expected resurrected")
	}
	if alive.RemovedAt != nil {
		t.Fatalf("expected removed_at cleared, got %v", alive.RemovedAt)
	}
	if alive.ID != before.ID {
		t.Fatalf("id changed across resurrection")
	}
}

func TestSync_MetaCarriesMechanismBody(t *testing.T) {
	prov := &stubProvider{
		refs: []cartridges.Ref{{ID: "popular"}},
		policies: map[string]map[string]cartridges.Manifest{
			"popular": {
				"r1": {
					RuleID: "r1", Version: 1, Name: "v1", Mechanism: "cedar",
					Body: map[string]any{
						"policy_file": "r1.cedar",
						"schema_file": "schema.json",
					},
				},
			},
		},
	}
	repo := newStubRepo()
	m := newSync(t, prov, repo)
	_, _ = m.Sync(context.Background())
	got, _ := repo.GetByNaturalKey(context.Background(), "popular", "r1")
	if got.Meta["policy_file"] != "r1.cedar" {
		t.Fatalf("meta=%+v", got.Meta)
	}
	if got.Meta["schema_file"] != "schema.json" {
		t.Fatalf("meta=%+v", got.Meta)
	}
}
