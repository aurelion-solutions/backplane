// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package llm

import (
	"fmt"
	"sort"
	"sync"
)

// Config carries the endpoint coordinates a protocol client needs to
// reach one backend. The same client serves many named backends — only
// these values differ between, say, a local llama-server and OpenAI.
type Config struct {
	BaseURL string
	APIKey  string
	Model   string
}

// Constructor builds a Provider for one protocol from a Config.
type Constructor func(Config) (Provider, error)

// Factory is a registry of Provider constructors keyed by wire protocol
// ("openai" | "anthropic" | "gemini"). A brand (qwen-local, deepseek,
// claude…) is not a registry entry — it is a Config handed to the
// protocol's constructor. One client per protocol, reused across
// brands.
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

// Register stores ctor under a protocol name. Overwrites any prior
// registration.
func (f *Factory) Register(protocol string, ctor Constructor) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ctors[protocol] = ctor
}

// Get constructs a fresh Provider for the given protocol, configured by
// cfg.
func (f *Factory) Get(protocol string, cfg Config) (Provider, error) {
	f.mu.RLock()
	ctor, ok := f.ctors[protocol]
	f.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("llm: protocol %q is not registered (known: %v)", protocol, f.Names())
	}
	return ctor(cfg)
}

// Names returns the registered protocol names in sorted order.
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
