// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package siem

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// DefaultFilePath is the relative path used when no override is supplied.
const DefaultFilePath = ".logs/aurelion.log.jsonl"

// FileSink is an append-only JSONL sink. One Event per line, UTF-8.
// Writes are serialised by an internal mutex; safe for concurrent Emit.
type FileSink struct {
	path string
	mu   sync.Mutex
}

// NewFileSink returns a sink that appends to path. The parent
// directory is created on first Emit, not at construction, so a
// missing directory does not fail startup.
func NewFileSink(path string) *FileSink {
	if path == "" {
		path = DefaultFilePath
	}
	return &FileSink{path: path}
}

// Emit implements Sink.
func (s *FileSink) Emit(_ context.Context, event Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("logsink/file: marshal: %w", err)
	}
	body = append(body, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("logsink/file: mkdir: %w", err)
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("logsink/file: open %q: %w", s.path, err)
	}
	defer f.Close()
	if _, err := f.Write(body); err != nil {
		return fmt.Errorf("logsink/file: write: %w", err)
	}
	return nil
}

// FileReader reads previously emitted Events from the same JSONL file.
// Returns up to `limit` of the most recent records.
type FileReader struct {
	path string
}

// NewFileReader returns a reader against path.
func NewFileReader(path string) *FileReader {
	if path == "" {
		path = DefaultFilePath
	}
	return &FileReader{path: path}
}

// Read implements Reader.
func (r *FileReader) Read(_ context.Context, limit int) ([]Event, error) {
	if limit <= 0 {
		return nil, nil
	}
	f, err := os.Open(r.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("logsink/file: open %q: %w", r.path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	var all []Event
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		all = append(all, ev)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("logsink/file: scan: %w", err)
	}
	if len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all, nil
}

// RegisterFile wires the "file" provider into f.
func RegisterFile(f *Factory, path string) {
	f.Register("file", func() (Sink, error) {
		return NewFileSink(path), nil
	})
}
