// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package cartridges

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcher_FirstScanSeedsState(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.yaml"), "x: 1")

	w := NewWatcher(dir, WatchSuffixes(".yaml"))
	changed, err := w.Changed()
	if err != nil {
		t.Fatalf("first changed: %v", err)
	}
	if changed {
		t.Fatalf("expected first scan to return false (seed)")
	}
}

func TestWatcher_DetectsContentChange(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.yaml")
	writeFile(t, p, "x: 1")

	w := NewWatcher(dir, WatchSuffixes(".yaml"))
	_, _ = w.Changed()

	// Bump mtime to ensure we don't race against same-second resolution.
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(p, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	changed, err := w.Changed()
	if err != nil {
		t.Fatalf("changed: %v", err)
	}
	if !changed {
		t.Fatalf("expected change after mtime bump")
	}
}

func TestWatcher_DetectsCreate(t *testing.T) {
	dir := t.TempDir()
	w := NewWatcher(dir, WatchSuffixes(".yaml"))
	_, _ = w.Changed()

	writeFile(t, filepath.Join(dir, "b.yaml"), "x: 2")
	changed, err := w.Changed()
	if err != nil {
		t.Fatalf("changed: %v", err)
	}
	if !changed {
		t.Fatalf("expected change after create")
	}
}

func TestWatcher_DetectsDelete(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.yaml")
	writeFile(t, p, "x: 1")

	w := NewWatcher(dir, WatchSuffixes(".yaml"))
	_, _ = w.Changed()

	if err := os.Remove(p); err != nil {
		t.Fatalf("remove: %v", err)
	}
	changed, err := w.Changed()
	if err != nil {
		t.Fatalf("changed: %v", err)
	}
	if !changed {
		t.Fatalf("expected change after delete")
	}
}

func TestWatcher_SuffixFilter(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.yaml"), "x: 1")
	writeFile(t, filepath.Join(dir, "ignored.txt"), "hello")

	w := NewWatcher(dir, WatchSuffixes(".yaml"))
	_, _ = w.Changed()

	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(filepath.Join(dir, "ignored.txt"), future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	changed, err := w.Changed()
	if err != nil {
		t.Fatalf("changed: %v", err)
	}
	if changed {
		t.Fatalf("did not expect change for filtered-out file")
	}
}

func TestWatcher_MissingRoot(t *testing.T) {
	w := NewWatcher(filepath.Join(t.TempDir(), "nope"))
	changed, err := w.Changed()
	if err != nil {
		t.Fatalf("changed on missing root: %v", err)
	}
	if changed {
		t.Fatalf("missing root should not report change")
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}
