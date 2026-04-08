// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package orchestrator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aurelion-solutions/backplane/internal/actions/noop"
	"github.com/aurelion-solutions/backplane/internal/core/cartridges"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/loader"
	"github.com/aurelion-solutions/backplane/internal/core/orchestrator/registry"
)

const echoYAML = `
pipeline:
  name: %s
  version: 1
  schema_version: 1
  args:
    type: object
    properties:
      subject:
        type: string
  steps:
    - name: echo
      engine: noop
      action: echo
      args:
        message: "hi ${args.subject}"
`

func buildCartridgesTree(t *testing.T, layouts map[string][]struct{ Name, Body string }) string {
	t.Helper()
	root := t.TempDir()
	for cart, files := range layouts {
		dir := filepath.Join(root, cart, "pipelines")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if err := os.WriteFile(filepath.Join(dir, f.Name), []byte(f.Body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}

func TestLoadFromCartridges_DefaultScansAllIDs(t *testing.T) {
	root := buildCartridgesTree(t, map[string][]struct{ Name, Body string }{
		"alpha": {{"a.yaml", `pipeline:
  name: a
  version: 1
  schema_version: 1
  steps: [{name: echo, engine: noop, action: echo}]
`}},
		"beta": {{"b.yaml", `pipeline:
  name: b
  version: 1
  schema_version: 1
  steps: [{name: echo, engine: noop, action: echo}]
`}},
	})
	p := cartridges.NewFilesystemProvider(root)
	reg := registry.New()
	noop.Register(reg)
	cat, err := LoadFromCartridges(p, &loader.Loader{Actions: reg}, nil)
	if err != nil {
		t.Fatalf("LoadFromCartridges: %v", err)
	}
	if cat.Get("a") == nil || cat.Get("b") == nil {
		t.Fatalf("missing pipelines in %v", cat.All())
	}
	want := []string{"alpha", "beta"}
	if !equal(cat.Sources(), want) {
		t.Fatalf("Sources = %v, want %v", cat.Sources(), want)
	}
}

func TestLoadFromCartridges_FailsOnDuplicateName(t *testing.T) {
	root := buildCartridgesTree(t, map[string][]struct{ Name, Body string }{
		"alpha": {{"p.yaml", `pipeline:
  name: shared
  version: 1
  schema_version: 1
  steps: [{name: echo, engine: noop, action: echo}]
`}},
		"beta": {{"p.yaml", `pipeline:
  name: shared
  version: 1
  schema_version: 1
  steps: [{name: echo, engine: noop, action: echo}]
`}},
	})
	p := cartridges.NewFilesystemProvider(root)
	reg := registry.New()
	noop.Register(reg)
	_, err := LoadFromCartridges(p, &loader.Loader{Actions: reg}, []string{"alpha", "beta"})
	if err == nil {
		t.Fatal("want duplicate-name error")
	}
}

func TestBuildMergedSchema_InjectsActionDefs(t *testing.T) {
	reg := registry.New()
	noop.Register(reg)
	schema, err := BuildMergedSchema(reg)
	if err != nil {
		t.Fatalf("BuildMergedSchema: %v", err)
	}
	defs := schema["$defs"].(map[string]any)
	args := defs["action_args"].(map[string]any)
	if _, ok := args["noop.echo"]; !ok {
		t.Fatalf("noop.echo missing from action_args: %v", args)
	}
	results := defs["action_results"].(map[string]any)
	if _, ok := results["noop.sleep"]; !ok {
		t.Fatalf("noop.sleep missing from action_results: %v", results)
	}
}

func TestBuildActionCatalogue_Sorted(t *testing.T) {
	reg := registry.New()
	noop.Register(reg)
	cat := BuildActionCatalogue(reg)
	if len(cat) != 2 {
		t.Fatalf("len = %d, want 2", len(cat))
	}
	if cat[0].Action != "echo" || cat[1].Action != "sleep" {
		t.Fatalf("catalogue order = [%s, %s]", cat[0].Action, cat[1].Action)
	}
	if !cat[0].Idempotent || !cat[1].Idempotent {
		t.Fatalf("idempotent flag lost")
	}
}

func equal(a, b []string) bool {
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
