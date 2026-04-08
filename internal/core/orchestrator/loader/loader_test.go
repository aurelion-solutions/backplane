// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package loader

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validPipeline = `
pipeline:
  name: smoke.echo
  version: 1
  schema_version: 1
  args:
    type: object
    required: [subject]
    properties:
      subject:
        type: string
  steps:
    - name: echo
      engine: noop
      action: echo
      args:
        message: "hello ${args.subject}"
    - name: tail
      engine: noop
      action: echo
      args:
        message: "after ${steps.echo.result.message}"
      requires: [echo]
`

func writeYAML(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "p.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestLoader_Valid(t *testing.T) {
	l := New()
	defn, err := l.LoadFile(writeYAML(t, validPipeline))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if defn.Name != "smoke.echo" {
		t.Fatalf("Name = %q, want smoke.echo", defn.Name)
	}
	if defn.Version != 1 || defn.SchemaVersion != 1 {
		t.Fatalf("versions = (%d, %d), want (1, 1)", defn.Version, defn.SchemaVersion)
	}
	if len(defn.Steps) != 2 {
		t.Fatalf("Steps = %d, want 2", len(defn.Steps))
	}
	if defn.ContentHash == "" || len(defn.ContentHash) != 64 {
		t.Fatalf("ContentHash = %q (len %d)", defn.ContentHash, len(defn.ContentHash))
	}
}

func TestLoader_ReservedName_Runs(t *testing.T) {
	body := strings.Replace(validPipeline, "name: smoke.echo", "name: runs", 1)
	_, err := New().LoadFile(writeYAML(t, body))
	if !errors.Is(err, ErrSchema) {
		t.Fatalf("want ErrSchema (reserved name), got %v", err)
	}
}

func TestLoader_ForwardRequires(t *testing.T) {
	body := `
pipeline:
  name: smoke.echo
  version: 1
  schema_version: 1
  steps:
    - name: a
      engine: noop
      action: echo
      requires: [b]
    - name: b
      engine: noop
      action: echo
`
	_, err := New().LoadFile(writeYAML(t, body))
	if !errors.Is(err, ErrRequiresOrder) {
		t.Fatalf("want ErrRequiresOrder, got %v", err)
	}
}

func TestLoader_UndeclaredArg(t *testing.T) {
	body := `
pipeline:
  name: smoke.echo
  version: 1
  schema_version: 1
  steps:
    - name: echo
      engine: noop
      action: echo
      args:
        message: "hi ${args.nope}"
`
	_, err := New().LoadFile(writeYAML(t, body))
	if !errors.Is(err, ErrTemplating) {
		t.Fatalf("want ErrTemplating, got %v", err)
	}
}

func TestLoader_StepRefOutsideRequires(t *testing.T) {
	body := `
pipeline:
  name: smoke.echo
  version: 1
  schema_version: 1
  steps:
    - name: a
      engine: noop
      action: echo
      args:
        m: "${steps.b.result.x}"
    - name: b
      engine: noop
      action: echo
`
	_, err := New().LoadFile(writeYAML(t, body))
	if !errors.Is(err, ErrTemplating) {
		t.Fatalf("want ErrTemplating, got %v", err)
	}
}

func TestLoader_WaitForEvent(t *testing.T) {
	body := `
pipeline:
  name: hitl.approval
  version: 1
  schema_version: 1
  args:
    type: object
    properties:
      case_id:
        type: string
  steps:
    - name: park
      type: wait_for_event
      event: approval.granted
      match:
        case_id: "${args.case_id}"
      timeout: 7d
`
	defn, err := New().LoadFile(writeYAML(t, body))
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if got := StepKind(defn.Steps[0]); got != StepWaitForEvent {
		t.Fatalf("StepKind = %q, want %q", got, StepWaitForEvent)
	}
}

func TestLoader_WaitForEvent_TimeoutRequired(t *testing.T) {
	body := `
pipeline:
  name: hitl.approval
  version: 1
  schema_version: 1
  steps:
    - name: park
      type: wait_for_event
      event: approval.granted
`
	_, err := New().LoadFile(writeYAML(t, body))
	if !errors.Is(err, ErrSchema) {
		t.Fatalf("want ErrSchema (missing timeout), got %v", err)
	}
}

func TestLoader_DuplicateScheduleTrigger(t *testing.T) {
	body := `
pipeline:
  name: nightly.scan
  version: 1
  schema_version: 1
  triggers:
    - type: schedule
      every: 1h
    - type: schedule
      cron: "0 0 * * *"
  steps:
    - name: a
      engine: noop
      action: echo
`
	_, err := New().LoadFile(writeYAML(t, body))
	if !errors.Is(err, ErrTrigger) {
		t.Fatalf("want ErrTrigger, got %v", err)
	}
}

func TestLoader_LoadDir_Duplicate(t *testing.T) {
	dir := t.TempDir()
	w := func(name, n string) {
		body := strings.Replace(validPipeline, "name: smoke.echo", "name: "+n, 1)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	w("a.yaml", "dup.same")
	w("b.yaml", "dup.same")
	_, err := New().LoadDir(dir)
	if !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("want ErrDuplicateName, got %v", err)
	}
}

func TestLoader_ActionLookup(t *testing.T) {
	body := `
pipeline:
  name: smoke.echo
  version: 1
  schema_version: 1
  steps:
    - name: a
      engine: noop
      action: echo
    - name: b
      engine: bogus
      action: nope
`
	l := &Loader{Actions: stubLookup{"noop": {"echo": true}}}
	_, err := l.LoadFile(writeYAML(t, body))
	if !errors.Is(err, ErrActionRef) {
		t.Fatalf("want ErrActionRef, got %v", err)
	}
}

// --- helpers --------------------------------------------------------

type stubLookup map[string]map[string]bool

func (s stubLookup) Has(engine, action string) bool {
	return s[engine][action]
}

// TestLoader_RealCartridges loads pipelines from the actual
// cartridges/journey/pipelines directory. Skipped when the file tree
// is missing (CI without the cartridges checkout).
func TestLoader_RealCartridges(t *testing.T) {
	dir := "/Users/michael/Desktop/Aurelion/code/cartridges/journey/pipelines"
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("cartridges dir absent: %v", err)
	}
	defs, err := New().LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir(journey): %v", err)
	}
	for name, defn := range defs {
		if defn.Name != name {
			t.Fatalf("name mismatch: key=%s defn.Name=%s", name, defn.Name)
		}
	}
	want := []string{"journey.joiner", "journey.leaver", "journey.on_leave", "journey.return_from_leave"}
	for _, n := range want {
		if _, ok := defs[n]; !ok {
			t.Fatalf("missing pipeline %q in %v", n, mapKeys(defs))
		}
	}
}

func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
