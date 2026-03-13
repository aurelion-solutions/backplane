// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package secretmanagers holds concrete secret.FullManager implementations.
// One file per backend (file, vault, akeyless, conjur, openbao, …). Each
// provider exposes a Register* helper that wires itself into secret.Factory.
package secretmanagers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/aurelion-solutions/backplane/internal/core/secret"
)

// File is a development-only secret store backed by a local JSON file.
// Secrets are stored in plain text — do not use in production.
//
// Values in the file may be either a JSON string or a nested JSON
// object. On Get, a nested object is re-encoded to a JSON string so
// callers always see the same string-shaped API. This lets you hand-
// edit .secrets.json without the ugly "{\"host\":...}" escaping.
//
// Set always writes a string value (the API takes string). Mixing
// hand-written objects and Set-written strings in the same file is
// fine — Get treats them uniformly.
//
// Live-read on every operation; writes are atomic via temp + rename.
// Safe for concurrent use within a process.
type File struct {
	path string
	mu   sync.Mutex
}

// NewFile returns a provider reading from / writing to path.
func NewFile(path string) *File {
	return &File{path: path}
}

// Get implements secret.Manager.
func (p *File) Get(key string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	data, err := p.load()
	if err != nil {
		return "", err
	}
	raw, ok := data[key]
	if !ok {
		return "", fmt.Errorf("%w: %q (file=%s)", secret.ErrNotFound, key, p.path)
	}
	return encodeValue(raw)
}

// Set implements secret.Mutator.
func (p *File) Set(key, value string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	data, err := p.load()
	if err != nil {
		return err
	}
	data[key] = value
	return p.save(data)
}

// Delete implements secret.Mutator.
func (p *File) Delete(key string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	data, err := p.load()
	if err != nil {
		return err
	}
	if _, ok := data[key]; !ok {
		return fmt.Errorf("%w: %q (file=%s)", secret.ErrNotFound, key, p.path)
	}
	delete(data, key)
	return p.save(data)
}

func (p *File) load() (map[string]any, error) {
	raw, err := os.ReadFile(p.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("secretmanagers/file: read %q: %w", p.path, err)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("secretmanagers/file: parse %q: %w", p.path, err)
	}
	if data == nil {
		data = map[string]any{}
	}
	return data, nil
}

func (p *File) save(data map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		return fmt.Errorf("secretmanagers/file: mkdir: %w", err)
	}
	body, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("secretmanagers/file: marshal: %w", err)
	}
	tmp := p.path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return fmt.Errorf("secretmanagers/file: write tmp: %w", err)
	}
	if err := os.Rename(tmp, p.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("secretmanagers/file: rename: %w", err)
	}
	return nil
}

// encodeValue normalises a raw JSON value into the string shape the
// secret.Manager API hands back. Strings pass through; everything else
// is re-encoded to JSON so callers can json.Unmarshal it as before.
func encodeValue(v any) (string, error) {
	if s, ok := v.(string); ok {
		return s, nil
	}
	body, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("secretmanagers/file: encode value: %w", err)
	}
	return string(body), nil
}

// RegisterFile wires the "file" provider into f.
func RegisterFile(f *Factory, path string) {
	f.Register("file", func() (secret.FullManager, error) {
		return NewFile(path), nil
	})
}
