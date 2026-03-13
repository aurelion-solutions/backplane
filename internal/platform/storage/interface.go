// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

// Package storage delivers data-lake batch storage. One package holds
// the Storage contract, the Factory registry, and every provider.
//
// Records are passed as []map[string]any. For very large batches this
// loads everything into memory; switch to iter.Seq when that hurts.
package storage

import (
	"context"
	"errors"
)

// ErrNotFound is returned when the requested storage_key is absent.
var ErrNotFound = errors.New("storage: batch not found")

// ErrNotImplemented is returned by stub providers.
var ErrNotImplemented = errors.New("storage: provider not implemented")

// Storage is the contract every data-lake backend implements.
type Storage interface {
	// WriteBatch persists records under datasetType and returns an
	// opaque storage_key for later retrieval.
	WriteBatch(ctx context.Context, datasetType string, records []map[string]any) (string, error)

	// ReadBatch loads the batch identified by storageKey.
	ReadBatch(ctx context.Context, storageKey string) ([]map[string]any, error)

	// DeleteBatch removes the batch identified by storageKey.
	DeleteBatch(ctx context.Context, storageKey string) error
}
