// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package cartridges

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// buildFixture writes a minimal cartridges tree under tmpRoot and
// returns the root path. Layout:
//
//	<root>/
//	  alpha/
//	    pipelines/
//	      first.yaml
//	      second.yaml
//	    policies/
//	      bucket/
//	        rule_one.meta.json
//	        rule_one.rego          (ignored — provider only reads .meta.json)
//	        rule_two.meta.json
//	  beta/
//	    policies/
//	      flat.meta.json
//	  empty/                       (no subdirs — listed but counts == 0)
//	  .hidden/                     (skipped)
func buildFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	mustMkdir := func(rel string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(root, rel), 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", rel, err)
		}
	}
	mustWrite := func(rel, body string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %q: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %q: %v", rel, err)
		}
	}

	mustMkdir("alpha/pipelines")
	mustWrite("alpha/pipelines/first.yaml", "pipeline: {}\n")
	mustWrite("alpha/pipelines/second.yaml", "pipeline: {}\n")
	mustWrite("alpha/pipelines/.gitkeep", "")

	mustWrite("alpha/policies/bucket/rule_one.meta.json",
		`{"rule_id":"alpha.one","version":1,"name":"One","mechanism":"generic"}`)
	mustWrite("alpha/policies/bucket/rule_one.rego", "package alpha.one\n")
	mustWrite("alpha/policies/bucket/rule_two.meta.json",
		`{"rule_id":"alpha.two","version":1,"name":"Two","mechanism":"generic"}`)

	mustWrite("beta/policies/flat.meta.json",
		`{"rule_id":"beta.flat","version":1,"name":"Flat","mechanism":"generic"}`)

	mustMkdir("empty")
	mustMkdir(".hidden")

	return root
}

func TestFilesystemProvider_List(t *testing.T) {
	p := NewFilesystemProvider(buildFixture(t))
	refs, err := p.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := make([]string, 0, len(refs))
	for _, r := range refs {
		got = append(got, r.ID)
	}
	want := []string{"alpha", "beta", "empty"}
	if !equalStrings(got, want) {
		t.Fatalf("List ids = %v, want %v", got, want)
	}
}

func TestFilesystemProvider_List_MissingRoot(t *testing.T) {
	p := NewFilesystemProvider(filepath.Join(t.TempDir(), "absent"))
	refs, err := p.List()
	if err != nil {
		t.Fatalf("missing root should not error, got %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("missing root should yield empty list, got %v", refs)
	}
}

func TestFilesystemProvider_Materialize_NotFound(t *testing.T) {
	p := NewFilesystemProvider(buildFixture(t))
	_, err := p.Materialize(Ref{ID: "no-such"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestFilesystemProvider_Policies(t *testing.T) {
	p := NewFilesystemProvider(buildFixture(t))
	got, err := p.Policies(Ref{ID: "alpha"})
	if err != nil {
		t.Fatalf("Policies(alpha): %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Policies(alpha) count = %d, want 2", len(got))
	}
	if _, ok := got["alpha.one"]; !ok {
		t.Fatalf("missing rule alpha.one in %v", got)
	}
	if got["alpha.one"].Mechanism != "generic" {
		t.Fatalf("alpha.one.mechanism = %q, want generic", got["alpha.one"].Mechanism)
	}
}

func TestFilesystemProvider_Policies_Empty(t *testing.T) {
	p := NewFilesystemProvider(buildFixture(t))
	got, err := p.Policies(Ref{ID: "empty"})
	if err != nil {
		t.Fatalf("Policies(empty): %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Policies(empty) = %v, want 0", got)
	}
}

func TestFilesystemProvider_Pipelines(t *testing.T) {
	p := NewFilesystemProvider(buildFixture(t))
	got, err := p.Pipelines(Ref{ID: "alpha"})
	if err != nil {
		t.Fatalf("Pipelines(alpha): %v", err)
	}
	bases := make([]string, 0, len(got))
	for _, path := range got {
		bases = append(bases, filepath.Base(path))
	}
	want := []string{"first.yaml", "second.yaml"}
	if !equalStrings(bases, want) {
		t.Fatalf("Pipelines(alpha) basenames = %v, want %v", bases, want)
	}
}

func TestFilesystemProvider_Pipelines_NoDir(t *testing.T) {
	p := NewFilesystemProvider(buildFixture(t))
	got, err := p.Pipelines(Ref{ID: "beta"})
	if err != nil {
		t.Fatalf("Pipelines(beta): %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Pipelines(beta) = %v, want 0", got)
	}
}

func TestLoadManifest_RejectsMissingFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.meta.json")
	if err := os.WriteFile(path, []byte(`{"rule_id":"x","version":1,"name":"X"}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := loadManifest(path)
	if !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("want ErrInvalidManifest (missing mechanism), got %v", err)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
