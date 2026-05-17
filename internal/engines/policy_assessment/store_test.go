// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policy_assessment

import (
	"context"
	"testing"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
)

type stubProvider struct {
	refs     []cartridges.Ref
	policies map[string]map[string]cartridges.Manifest
}

func (s *stubProvider) List() ([]cartridges.Ref, error)              { return s.refs, nil }
func (s *stubProvider) Materialize(_ cartridges.Ref) (string, error) { return "", nil }
func (s *stubProvider) Policies(ref cartridges.Ref) (map[string]cartridges.Manifest, error) {
	return s.policies[ref.ID], nil
}
func (s *stubProvider) Pipelines(_ cartridges.Ref) ([]string, error) { return nil, nil }
func (s *stubProvider) Apps(_ cartridges.Ref) (map[string]cartridges.AppCartridge, error) {
	return nil, nil
}

func TestStore_SelectByFacets(t *testing.T) {
	store := NewStore()
	prov := &stubProvider{
		refs: []cartridges.Ref{{ID: "alpha"}},
		policies: map[string]map[string]cartridges.Manifest{
			"alpha": {
				"r1": {RuleID: "r1", Version: 1, Name: "untagged", Mechanism: "cedar"},
				"r2": {RuleID: "r2", Version: 1, Name: "authn-only", Mechanism: "cedar",
					Tags: []string{"authn"}},
				"r3": {RuleID: "r3", Version: 1, Name: "saml-eu", Mechanism: "cedar",
					Tags: []string{"authn", "transport:saml", "geo:eu"}},
				"r4": {RuleID: "r4", Version: 1, Name: "scan-sox", Mechanism: "sod",
					Tags: []string{"scan", "framework:sox"}},
			},
		},
	}
	n, err := store.Reload(context.Background(), prov)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if n != 4 {
		t.Fatalf("reload count=%d want 4", n)
	}

	cases := []struct {
		name   string
		facets []string
		want   []string // rule ids in expected order doesn't matter; matched by set
	}{
		{
			name:   "no facets only matches untagged",
			facets: nil,
			want:   []string{"r1"},
		},
		{
			name:   "authn alone matches untagged + authn-only",
			facets: []string{"authn"},
			want:   []string{"r1", "r2"},
		},
		{
			name:   "saml eu authn matches all authn rules",
			facets: []string{"authn", "transport:saml", "geo:eu"},
			want:   []string{"r1", "r2", "r3"},
		},
		{
			name:   "scan sox matches sod rule only",
			facets: []string{"scan", "framework:sox"},
			want:   []string{"r1", "r4"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := store.SelectByFacets(c.facets)
			gotIDs := map[string]bool{}
			for _, e := range got {
				gotIDs[e.Manifest.RuleID] = true
			}
			if len(gotIDs) != len(c.want) {
				t.Fatalf("count: got %v want %v", gotIDs, c.want)
			}
			for _, id := range c.want {
				if !gotIDs[id] {
					t.Fatalf("missing %s in %v", id, gotIDs)
				}
			}
		})
	}
}

func TestStore_SelectByMechanism(t *testing.T) {
	store := NewStore()
	prov := &stubProvider{
		refs: []cartridges.Ref{{ID: "alpha"}},
		policies: map[string]map[string]cartridges.Manifest{
			"alpha": {
				"r1": {RuleID: "r1", Version: 1, Name: "c", Mechanism: "cedar"},
				"r2": {RuleID: "r2", Version: 1, Name: "s", Mechanism: "sod"},
				"r3": {RuleID: "r3", Version: 1, Name: "l", Mechanism: "llm_classification"},
			},
		},
	}
	_, _ = store.Reload(context.Background(), prov)

	cedar := store.SelectByMechanism("cedar")
	if len(cedar) != 1 || cedar[0].Manifest.RuleID != "r1" {
		t.Fatalf("cedar=%+v", cedar)
	}
	cedarSod := store.SelectByMechanism("cedar", "sod")
	if len(cedarSod) != 2 {
		t.Fatalf("cedarSod=%+v", cedarSod)
	}
}
