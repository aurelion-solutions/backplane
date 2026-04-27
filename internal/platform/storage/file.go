// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package storage

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// DefaultBasePath is the relative directory used when no override is
// supplied. The lake lives one level above each binary's cwd so it
// can be shared by every subrepo in the monorepo (kernel, backplane,
// future runtimes) without polluting any individual project tree.
const DefaultBasePath = "../.lake"

// File is a development-only data-lake backend. Batches are written
// as JSONL (one record per line) under <base>/<datasetType>/<uuid>.jsonl.
type File struct {
	base string
}

// NewFile returns a backend rooted at base. base is created on first
// WriteBatch; absence at construction is not an error.
func NewFile(base string) *File {
	if base == "" {
		base = DefaultBasePath
	}
	return &File{base: base}
}

// WriteBatch implements Storage.
func (s *File) WriteBatch(_ context.Context, datasetType string, records []map[string]any) (string, error) {
	if err := validateDatasetType(datasetType); err != nil {
		return "", err
	}
	key := uuid.NewString()
	rel := filepath.Join(datasetType, key+".jsonl")
	path := filepath.Join(s.base, rel)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("storage/file: mkdir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("storage/file: create %q: %w", path, err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, rec := range records {
		body, err := json.Marshal(rec)
		if err != nil {
			return "", fmt.Errorf("storage/file: marshal: %w", err)
		}
		if _, err := w.Write(body); err != nil {
			return "", fmt.Errorf("storage/file: write: %w", err)
		}
		if err := w.WriteByte('\n'); err != nil {
			return "", fmt.Errorf("storage/file: write: %w", err)
		}
	}
	if err := w.Flush(); err != nil {
		return "", fmt.Errorf("storage/file: flush: %w", err)
	}
	return datasetType + "/" + key, nil
}

// ReadBatch implements Storage.
func (s *File) ReadBatch(_ context.Context, storageKey string) ([]map[string]any, error) {
	if err := validateStorageKey(storageKey); err != nil {
		return nil, err
	}
	path := filepath.Join(s.base, storageKey+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %q", ErrNotFound, storageKey)
		}
		return nil, fmt.Errorf("storage/file: open %q: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	var out []map[string]any
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil, fmt.Errorf("storage/file: parse line: %w", err)
		}
		out = append(out, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("storage/file: scan: %w", err)
	}
	return out, nil
}

// DeleteBatch implements Storage.
func (s *File) DeleteBatch(_ context.Context, storageKey string) error {
	if err := validateStorageKey(storageKey); err != nil {
		return err
	}
	path := filepath.Join(s.base, storageKey+".jsonl")
	if err := os.Remove(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: %q", ErrNotFound, storageKey)
		}
		return fmt.Errorf("storage/file: remove %q: %w", path, err)
	}
	return nil
}

// validateDatasetType refuses path-traversal in dataset name.
func validateDatasetType(t string) error {
	if t == "" || strings.Contains(t, "..") || strings.ContainsAny(t, `/\`) {
		return fmt.Errorf("storage/file: invalid dataset_type %q", t)
	}
	return nil
}

// validateStorageKey refuses absolute paths and traversal in storage_key.
func validateStorageKey(k string) error {
	if k == "" || strings.Contains(k, "..") || strings.HasPrefix(k, "/") {
		return fmt.Errorf("storage/file: invalid storage_key %q", k)
	}
	return nil
}

// RegisterFile wires the "file" provider into f.
func RegisterFile(f *Factory, base string) {
	f.Register("file", func() (Storage, error) {
		return NewFile(base), nil
	})
}
