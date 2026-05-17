// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package cartridges

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// pipelinesSubdir + policiesSubdir are the conventional layout inside a
// cartridge directory.
const (
	pipelinesSubdir = "pipelines"
	policiesSubdir  = "policies"
)

// FilesystemProvider treats every top-level subdirectory of Root as one
// cartridge. The directory name is the cartridge id; no manifest at the
// cartridge level is required.
//
// Files starting with "." are skipped so .gitkeep / .DS_Store don't
// pollute the list.
type FilesystemProvider struct {
	root string
}

// NewFilesystemProvider returns a provider rooted at root. The
// directory does NOT have to exist at construction; List returns an
// empty slice when it's missing.
func NewFilesystemProvider(root string) *FilesystemProvider {
	return &FilesystemProvider{root: root}
}

// Root returns the absolute or relative root directory the provider
// scans. Exposed for diagnostics + the routes layer.
func (p *FilesystemProvider) Root() string {
	return p.root
}

// List implements Provider.
func (p *FilesystemProvider) List() ([]Ref, error) {
	entries, err := os.ReadDir(p.root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []Ref{}, nil
		}
		return nil, fmt.Errorf("cartridges: read %q: %w", p.root, err)
	}
	out := make([]Ref, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		out = append(out, Ref{ID: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Materialize implements Provider.
func (p *FilesystemProvider) Materialize(ref Ref) (string, error) {
	if ref.ID == "" {
		return "", fmt.Errorf("%w: empty cartridge id", ErrNotFound)
	}
	path := filepath.Join(p.root, ref.ID)
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%w: %q", ErrNotFound, ref.ID)
	}
	return path, nil
}

// Policies implements Provider.
//
// Each manifest's BasePath is filled in with the absolute path of the
// .meta.json file (so handlers can resolve their own mechanism-specific
// sibling files: .cedar, .prompt, .yaml, …). The platform layer makes
// no assumption about which sibling files should or must exist —
// that's the mechanism handler's job.
func (p *FilesystemProvider) Policies(ref Ref) (map[string]Manifest, error) {
	root, err := p.Materialize(ref)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, policiesSubdir)
	out := map[string]Manifest{}

	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return filepath.SkipAll
			}
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".meta.json") {
			return nil
		}
		m, err := loadManifest(path)
		if err != nil {
			return err
		}
		m.BasePath = path
		if existing, dup := out[m.RuleID]; dup {
			return fmt.Errorf("%w: duplicate rule_id %q (in %q and %q)",
				ErrInvalidManifest, m.RuleID, existing.Name, path)
		}
		out[m.RuleID] = m
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, fs.ErrNotExist) {
		return nil, walkErr
	}
	return out, nil
}

// Apps implements Provider.
func (p *FilesystemProvider) Apps(ref Ref) (map[string]AppCartridge, error) {
	root, err := p.Materialize(ref)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, appsSubdir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]AppCartridge{}, nil
		}
		return nil, fmt.Errorf("cartridges: read %q: %w", dir, err)
	}
	out := map[string]AppCartridge{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		app, err := loadAppCartridge(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		if app.Manifest.ID != name {
			return nil, fmt.Errorf("%w: %s: manifest.id %q does not match directory name %q",
				ErrInvalidApp, app.BasePath, app.Manifest.ID, name)
		}
		if existing, dup := out[app.Manifest.ID]; dup {
			return nil, fmt.Errorf("%w: duplicate app id %q (in %s and %s)",
				ErrInvalidApp, app.Manifest.ID, existing.BasePath, app.BasePath)
		}
		out[app.Manifest.ID] = app
	}
	return out, nil
}

// Pipelines implements Provider.
func (p *FilesystemProvider) Pipelines(ref Ref) ([]string, error) {
	root, err := p.Materialize(ref)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, pipelinesSubdir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("cartridges: read %q: %w", dir, err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".yaml") {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	sort.Strings(out)
	return out, nil
}
