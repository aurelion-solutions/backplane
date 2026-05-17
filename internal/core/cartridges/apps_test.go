// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package cartridges

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validManifest, validAccount and validDescriptor are the canonical
// minimal YAML payloads used by tests. Individual cases substitute
// one of them with a broken variant to assert each validation rule.
const (
	validManifest = `
id: microsoft_ad
name: Microsoft Active Directory
version: 0.1.0
connector: ad
config:
  domain: ~
  ou_active: "OU=Users"
`
	validAccount = `
states: [not_exist, pending, active]
initial_state: not_exist
transitions:
  - { from: not_exist, to: pending }
  - { from: pending,   to: active }
`
	validDescriptor = `
fields:
  userPrincipalName:
    template: "{{ .Principal.Firstname }}@{{ .Application.Domain }}"
    transforms: [lower]
    on_collision: username_numeric_suffix
  userAccountControl:
    by_state:
      active: 512
      pending: 546
`
)

// writeApp materialises an app cartridge directory under root with
// the three files. Empty body skips the file (lets a test exercise
// "missing file" paths).
func writeApp(t *testing.T, root, id, manifest, account, descriptor string) {
	t.Helper()
	dir := filepath.Join(root, "apps", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", dir, err)
	}
	write := func(name, body string) {
		if body == "" {
			return
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write %q: %v", name, err)
		}
	}
	write(appManifestFile, manifest)
	write(appAccountFile, account)
	write(appDescriptorFile, descriptor)
}

func TestFilesystemProvider_Apps_Happy(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "popular")
	writeApp(t, bundle, "microsoft_ad", validManifest, validAccount, validDescriptor)

	p := NewFilesystemProvider(root)
	apps, err := p.Apps(Ref{ID: "popular"})
	if err != nil {
		t.Fatalf("Apps: %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("Apps count = %d, want 1", len(apps))
	}

	app, ok := apps["microsoft_ad"]
	if !ok {
		t.Fatalf("missing app microsoft_ad in %v", apps)
	}
	if app.Manifest.Connector != "ad" {
		t.Fatalf("connector = %q, want ad", app.Manifest.Connector)
	}
	if app.Account.InitialState != "not_exist" {
		t.Fatalf("initial_state = %q, want not_exist", app.Account.InitialState)
	}
	if len(app.Account.Transitions) != 2 {
		t.Fatalf("transitions count = %d, want 2", len(app.Account.Transitions))
	}

	upn, ok := app.Descriptor.Fields["userPrincipalName"]
	if !ok {
		t.Fatalf("missing field userPrincipalName")
	}
	if upn.OnCollision != "username_numeric_suffix" {
		t.Fatalf("on_collision = %q, want username_numeric_suffix", upn.OnCollision)
	}

	uac, ok := app.Descriptor.Fields["userAccountControl"]
	if !ok {
		t.Fatalf("missing field userAccountControl")
	}
	if uac.ByState["active"] != 512 {
		t.Fatalf("uac.by_state[active] = %v, want 512", uac.ByState["active"])
	}

	if app.BasePath == "" || !strings.HasSuffix(app.BasePath, "microsoft_ad") {
		t.Fatalf("BasePath = %q, want suffix microsoft_ad", app.BasePath)
	}
}

func TestFilesystemProvider_Apps_NoDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "popular"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	p := NewFilesystemProvider(root)
	apps, err := p.Apps(Ref{ID: "popular"})
	if err != nil {
		t.Fatalf("Apps: %v", err)
	}
	if len(apps) != 0 {
		t.Fatalf("Apps without apps/ dir should be empty, got %v", apps)
	}
}

func TestFilesystemProvider_Apps_DirNameMismatch(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "popular")
	writeApp(t, bundle, "wrong_dir", validManifest, validAccount, validDescriptor)

	p := NewFilesystemProvider(root)
	_, err := p.Apps(Ref{ID: "popular"})
	if !errors.Is(err, ErrInvalidApp) {
		t.Fatalf("want ErrInvalidApp, got %v", err)
	}
}

func TestFilesystemProvider_Apps_InvalidCases(t *testing.T) {
	cases := []struct {
		name       string
		manifest   string
		account    string
		descriptor string
	}{
		{
			name:       "missing manifest connector",
			manifest:   "id: microsoft_ad\nname: AD\nversion: 0.1.0\n",
			account:    validAccount,
			descriptor: validDescriptor,
		},
		{
			name:     "initial_state not in states",
			manifest: validManifest,
			account: `
states: [a, b]
initial_state: c
transitions: []
`,
			descriptor: validDescriptor,
		},
		{
			name:     "transition references unknown state",
			manifest: validManifest,
			account: `
states: [a, b]
initial_state: a
transitions:
  - { from: a, to: c }
`,
			descriptor: validDescriptor,
		},
		{
			name:     "descriptor field has neither template nor by_state",
			manifest: validManifest,
			account:  validAccount,
			descriptor: `
fields:
  bad:
    transforms: [lower]
`,
		},
		{
			name:     "descriptor field has both template and by_state",
			manifest: validManifest,
			account:  validAccount,
			descriptor: `
fields:
  bad:
    template: "x"
    by_state:
      active: "y"
`,
		},
		{
			name:     "by_state references unknown state",
			manifest: validManifest,
			account:  validAccount,
			descriptor: `
fields:
  bad:
    by_state:
      nonsense: 1
`,
		},
		{
			name:       "broken YAML",
			manifest:   "id: microsoft_ad\nname: [unbalanced\n",
			account:    validAccount,
			descriptor: validDescriptor,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			bundle := filepath.Join(root, "popular")
			writeApp(t, bundle, "microsoft_ad", tc.manifest, tc.account, tc.descriptor)

			p := NewFilesystemProvider(root)
			_, err := p.Apps(Ref{ID: "popular"})
			if !errors.Is(err, ErrInvalidApp) {
				t.Fatalf("want ErrInvalidApp, got %v", err)
			}
		})
	}
}

func TestFilesystemProvider_Apps_MissingFile(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "popular")
	// account.yaml deliberately omitted
	writeApp(t, bundle, "microsoft_ad", validManifest, "", validDescriptor)

	p := NewFilesystemProvider(root)
	_, err := p.Apps(Ref{ID: "popular"})
	if !errors.Is(err, ErrInvalidApp) {
		t.Fatalf("want ErrInvalidApp, got %v", err)
	}
}

func TestFilesystemProvider_Apps_SkipsHidden(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "popular")
	writeApp(t, bundle, "microsoft_ad", validManifest, validAccount, validDescriptor)

	if err := os.MkdirAll(filepath.Join(bundle, "apps", ".hidden"), 0o755); err != nil {
		t.Fatalf("mkdir hidden: %v", err)
	}

	p := NewFilesystemProvider(root)
	apps, err := p.Apps(Ref{ID: "popular"})
	if err != nil {
		t.Fatalf("Apps: %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("hidden dir should be skipped, got %v", apps)
	}
}
