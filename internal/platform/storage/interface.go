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

// Candidate is one (external_id, hash) pair that a writer is about
// to commit. Used as input to AntiJoin.
type Candidate struct {
	ExternalID string
	Hash       string
}

// AntiJoinResult is the categorised verdict for a set of Candidates.
// NewIDs are external_ids that have no row at all in the lake yet;
// ChangedIDs are external_ids whose latest hash differs from the
// candidate's. Anything in the input not listed in either is
// unchanged.
type AntiJoinResult struct {
	NewIDs     []string
	ChangedIDs []string
}

// Storage is the contract every data-lake backend implements.
type Storage interface {
	// WriteBatch persists records under datasetType and returns an
	// opaque storage_key for later retrieval. Each record is a
	// pre-shaped map with three top-level keys: external_id, meta
	// (backplane-added: hash, committed_at, correlation_id), and
	// payload (whatever the source sent verbatim).
	WriteBatch(ctx context.Context, datasetType string, records []map[string]any) (string, error)

	// ReadBatch loads the batch identified by storageKey.
	ReadBatch(ctx context.Context, storageKey string) ([]map[string]any, error)

	// DeleteBatch removes the batch identified by storageKey.
	DeleteBatch(ctx context.Context, storageKey string) error

	// AntiJoin classifies a set of candidate (external_id, hash)
	// pairs against the latest revision per external_id already in
	// the lake for datasetType. Pure read — does not mutate the
	// lake.
	AntiJoin(ctx context.Context, datasetType string, candidates []Candidate) (AntiJoinResult, error)
}
