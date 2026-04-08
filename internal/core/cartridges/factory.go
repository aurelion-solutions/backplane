// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package cartridges

import (
	"fmt"
	"sort"
	"sync"
)

// Constructor builds a Provider. Provider registrations supply one.
type Constructor func() (Provider, error)

// Factory is a registry of named Provider constructors, mirroring the
// pattern used by storage.Factory / siem.Factory.
//
// Safe for concurrent use.
type Factory struct {
	mu    sync.RWMutex
	ctors map[string]Constructor
}

// NewFactory returns an empty Factory.
func NewFactory() *Factory {
	return &Factory{ctors: make(map[string]Constructor)}
}

// Register stores ctor under name. Overwrites any prior registration.
func (f *Factory) Register(name string, ctor Constructor) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ctors[name] = ctor
}

// Get constructs a fresh Provider for the named source.
func (f *Factory) Get(name string) (Provider, error) {
	f.mu.RLock()
	ctor, ok := f.ctors[name]
	f.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("cartridges: provider %q is not registered (known: %v)", name, f.Names())
	}
	return ctor()
}

// Names returns the registered provider names in sorted order.
func (f *Factory) Names() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]string, 0, len(f.ctors))
	for k := range f.ctors {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// RegisterFilesystem wires the "filesystem" provider rooted at root.
func RegisterFilesystem(f *Factory, root string) {
	f.Register("filesystem", func() (Provider, error) {
		return NewFilesystemProvider(root), nil
	})
}
