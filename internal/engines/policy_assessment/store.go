// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package policy_assessment

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync/atomic"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
)

// Entry is one cartridge-loaded policy in the in-memory store.
//
// CartridgeRef + Manifest are everything the dispatcher needs to build
// a Request: Mechanism routes to the handler, Body carries the
// mechanism-specific payload, BasePath anchors sibling resolution.
type Entry struct {
	CartridgeRef string
	Manifest     cartridges.Manifest
}

// Store is the in-memory catalogue of every cartridge-defined policy
// the engine knows about. Read paths (All, SelectByFacets) are
// goroutine-safe via atomic snapshot pointer; reload swaps the whole
// slice atomically.
type Store struct {
	entries atomic.Pointer[[]Entry]
}

// NewStore returns an empty Store. Use Reload (or LoadFromCartridges)
// to populate it.
func NewStore() *Store {
	s := &Store{}
	empty := []Entry{}
	s.entries.Store(&empty)
	return s
}

// All returns every entry currently in the store. The returned slice
// is effectively immutable from the caller's perspective — Reload
// publishes a new slice via atomic swap; callers reading All() during
// reload either see the previous snapshot or the next one, never a
// mixture.
func (s *Store) All() []Entry {
	p := s.entries.Load()
	if p == nil {
		return nil
	}
	return *p
}

// SelectByFacets returns entries whose tags are a subset of the
// supplied request facets — the coarse pre-filter.
//
// Matching rule: every tag in Manifest.Tags must appear in facets.
// A policy with no tags matches every request (treated as "applicable
// to anything" by default). Facets are opaque strings — semantics like
// "geo:eu vs geo:DE" are the caller's responsibility (the caller can
// supply both when DE is in EU).
func (s *Store) SelectByFacets(facets []string) []Entry {
	all := s.All()
	if len(all) == 0 {
		return nil
	}
	facetSet := make(map[string]struct{}, len(facets))
	for _, f := range facets {
		facetSet[f] = struct{}{}
	}
	out := make([]Entry, 0, len(all))
	for _, e := range all {
		if tagsSubset(e.Manifest.Tags, facetSet) {
			out = append(out, e)
		}
	}
	return out
}

// SelectByMechanism returns every entry whose Manifest.Mechanism is in
// the supplied set. Useful when a caller (policy-assessment action) wants to
// iterate over one mechanism class without facet filtering.
func (s *Store) SelectByMechanism(mechanisms ...string) []Entry {
	if len(mechanisms) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(mechanisms))
	for _, m := range mechanisms {
		allowed[m] = struct{}{}
	}
	all := s.All()
	out := make([]Entry, 0, len(all))
	for _, e := range all {
		if _, ok := allowed[e.Manifest.Mechanism]; ok {
			out = append(out, e)
		}
	}
	return out
}

// Reload rebuilds the in-memory store from the supplied cartridges
// Provider. Failure leaves the previous snapshot in effect — the
// engine keeps serving the last good catalogue.
//
// Duplicate (cartridge_ref, rule_id) within one cartridge is caught by
// Provider.Policies already; cross-cartridge duplicates of the same
// rule_id are allowed (different cartridges, different namespaces).
func (s *Store) Reload(ctx context.Context, provider cartridges.Provider) (int, error) {
	if provider == nil {
		return 0, errors.New("policy_assessment.Store: nil provider")
	}
	refs, err := provider.List()
	if err != nil {
		return 0, fmt.Errorf("policy_assessment.Store: list cartridges: %w", err)
	}
	out := make([]Entry, 0, 64)
	for _, ref := range refs {
		manifests, err := provider.Policies(ref)
		if err != nil {
			return 0, fmt.Errorf("policy_assessment.Store: cartridge %q: %w", ref.ID, err)
		}
		ids := make([]string, 0, len(manifests))
		for id := range manifests {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			out = append(out, Entry{
				CartridgeRef: ref.ID,
				Manifest:     manifests[id],
			})
		}
	}
	s.entries.Store(&out)
	return len(out), nil
}

// MatchFacets is a stand-alone matcher exposed for tests / external
// callers that have an Entry slice in hand (e.g. a snapshot pulled
// from PG mirror, not the in-memory store).
func MatchFacets(entries []Entry, facets []string) []Entry {
	if len(entries) == 0 {
		return nil
	}
	facetSet := make(map[string]struct{}, len(facets))
	for _, f := range facets {
		facetSet[f] = struct{}{}
	}
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if tagsSubset(e.Manifest.Tags, facetSet) {
			out = append(out, e)
		}
	}
	return out
}

func tagsSubset(tags []string, facetSet map[string]struct{}) bool {
	for _, t := range tags {
		if _, ok := facetSet[t]; !ok {
			return false
		}
	}
	return true
}
