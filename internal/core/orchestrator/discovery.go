// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package orchestrator

import (
	"fmt"
	"path/filepath"
	"sort"
	"sync"

	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/loader"
)

// Catalog is the in-memory snapshot of every pipeline definition
// discovered, keyed by name. Reads (Get / All / Sources) are
// goroutine-safe; Reload swaps state under a write lock.
//
// Worker (and any other consumer that holds a *Catalog) sees a fresh
// snapshot the next time it calls Get / All — no need for a pointer
// swap at the consumer level.
type Catalog struct {
	mu      sync.RWMutex
	defs    map[string]*loader.Definition
	sources []string // cartridge ids in the order they were scanned
}

// NewCatalog returns an empty catalog. Useful for tests.
func NewCatalog() *Catalog {
	return &Catalog{defs: map[string]*loader.Definition{}}
}

// Get returns the definition for name, or nil when absent.
func (c *Catalog) Get(name string) *loader.Definition {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.defs[name]
}

// All returns every definition sorted by name.
func (c *Catalog) All() []*loader.Definition {
	c.mu.RLock()
	defer c.mu.RUnlock()
	names := make([]string, 0, len(c.defs))
	for n := range c.defs {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*loader.Definition, 0, len(names))
	for _, n := range names {
		out = append(out, c.defs[n])
	}
	return out
}

// Sources returns the cartridge ids the catalog was built from.
func (c *Catalog) Sources() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, len(c.sources))
	copy(out, c.sources)
	return out
}

// Reload re-scans every cartridge through the supplied provider and
// swaps the catalog's contents in place. Returns without mutation if
// the scan fails — the previous catalogue stays in effect.
//
// Pass an empty cartridgeIDs to re-scan everything the provider
// knows; same default as LoadFromCartridges.
func (c *Catalog) Reload(p cartridges.Provider, l *loader.Loader, cartridgeIDs []string) error {
	fresh, err := LoadFromCartridges(p, l, cartridgeIDs)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.defs = fresh.defs
	c.sources = fresh.sources
	c.mu.Unlock()
	return nil
}

// LoadFromCartridges scans every cartridge id in cartridgeIDs through
// the supplied provider and loads pipelines from
// <cartridge>/pipelines/. Duplicate pipeline.name across cartridges
// fails fast (loader.ErrDuplicateName).
//
// When cartridgeIDs is empty, every cartridge the provider knows about
// is scanned in sorted-id order.
func LoadFromCartridges(p cartridges.Provider, l *loader.Loader, cartridgeIDs []string) (*Catalog, error) {
	if l == nil {
		l = loader.New()
	}
	ids := cartridgeIDs
	if len(ids) == 0 {
		refs, err := p.List()
		if err != nil {
			return nil, fmt.Errorf("orchestrator/discovery: list cartridges: %w", err)
		}
		for _, ref := range refs {
			ids = append(ids, ref.ID)
		}
	}
	dirs := make([]string, 0, len(ids))
	for _, id := range ids {
		root, err := p.Materialize(cartridges.Ref{ID: id})
		if err != nil {
			return nil, fmt.Errorf("orchestrator/discovery: materialize %q: %w", id, err)
		}
		dirs = append(dirs, filepath.Join(root, "pipelines"))
	}
	defs, err := l.LoadMany(dirs)
	if err != nil {
		return nil, fmt.Errorf("orchestrator/discovery: %w", err)
	}
	return &Catalog{defs: defs, sources: ids}, nil
}
