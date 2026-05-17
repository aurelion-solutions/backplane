// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package cartridges

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DefaultPollInterval is the mtime-poll cadence used by both the
// backplane sync loop and per-process consumer reloads. 5 seconds is
// the design baseline picked in M2: short enough to feel "live", long
// enough that polling cost is negligible.
const DefaultPollInterval = 5 * time.Second

// Watcher detects file changes under a cartridges root by polling
// mtimes. Goroutine-safe — call Run from a single goroutine but
// Changed() can be called from anywhere.
//
// Watcher is intentionally minimal: it does not parse anything, does
// not know about cartridge structure, and does not coalesce events
// (the poll cadence is the coalescing window). Consumers wire it to
// whatever reload logic they own — registry rebuild, pipeline catalog
// reload, PG sync tick.
type Watcher struct {
	root     string
	suffixes []string
	log      *slog.Logger

	mu    sync.Mutex
	state map[string]time.Time
}

// WatcherOption tweaks the watcher's behavior.
type WatcherOption func(*Watcher)

// WatchSuffixes restricts the scan to files whose names end with one
// of the given suffixes. Empty list (the default) tracks every
// regular file. Hidden files (leading ".") are always skipped.
func WatchSuffixes(suffixes ...string) WatcherOption {
	return func(w *Watcher) {
		out := make([]string, 0, len(suffixes))
		for _, s := range suffixes {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		w.suffixes = out
	}
}

// WatchLogger sets a structured logger for diagnostics. Defaults to
// slog.Default() when unset.
func WatchLogger(log *slog.Logger) WatcherOption {
	return func(w *Watcher) { w.log = log }
}

// NewWatcher builds a Watcher rooted at root. The internal state map
// is left nil so the first Changed() seeds without reporting a diff.
func NewWatcher(root string, opts ...WatcherOption) *Watcher {
	w := &Watcher{
		root: root,
		log:  slog.Default(),
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Changed scans the root and returns true when any tracked file's
// mtime differs from the last call, or files appeared / disappeared.
//
// The first call after construction (or after Reset) seeds the state
// and returns false — the watcher is not a "fresh data" signal, it is
// a "has something changed since the last time you asked" signal.
// Consumers should perform their initial load separately.
func (w *Watcher) Changed() (bool, error) {
	current, err := w.scan()
	if err != nil {
		return false, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.state == nil {
		w.state = current
		return false, nil
	}
	changed := diff(w.state, current)
	w.state = current
	return changed, nil
}

// Reset drops the in-memory state so the next Changed() call seeds
// fresh — useful in tests.
func (w *Watcher) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.state = nil
}

// Run polls every interval until ctx is cancelled. When Changed
// returns true onChange is invoked; failures are logged and the loop
// continues.
//
// The initial seed scan happens inside the first tick — onChange is
// NOT called for it. Consumers should perform their boot-time load
// before calling Run.
func (w *Watcher) Run(ctx context.Context, interval time.Duration, onChange func(ctx context.Context) error) error {
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	// Seed state up-front so the first real tick can already
	// distinguish "no change" from "first scan".
	if _, err := w.Changed(); err != nil {
		w.log.Warn("cartridges watcher seed scan failed",
			slog.String("root", w.root), slog.Any("err", err))
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
		changed, err := w.Changed()
		if err != nil {
			w.log.Warn("cartridges watcher scan failed",
				slog.String("root", w.root), slog.Any("err", err))
			continue
		}
		if !changed {
			continue
		}
		if err := onChange(ctx); err != nil {
			w.log.Warn("cartridges watcher reload failed",
				slog.String("root", w.root), slog.Any("err", err))
		}
	}
}

func (w *Watcher) scan() (map[string]time.Time, error) {
	out := map[string]time.Time{}
	err := filepath.WalkDir(w.root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return filepath.SkipAll
			}
			return walkErr
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") && path != w.root {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !w.matchesSuffix(name) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		out[path] = info.ModTime()
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return out, nil
}

func (w *Watcher) matchesSuffix(name string) bool {
	if len(w.suffixes) == 0 {
		return true
	}
	for _, s := range w.suffixes {
		if strings.HasSuffix(name, s) {
			return true
		}
	}
	return false
}

func diff(prev, current map[string]time.Time) bool {
	if len(prev) != len(current) {
		return true
	}
	for path, mtime := range current {
		old, ok := prev[path]
		if !ok || !old.Equal(mtime) {
			return true
		}
	}
	return false
}
