// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package inventory_ingest

import "errors"

// ErrNotFound is returned when a batch audit row lookup misses.
var ErrNotFound = errors.New("inventory_ingest: batch not found")

// ErrInvalidEnvelope is returned when the envelope itself is malformed
// (missing source, missing dataset_type, oversized fields).
var ErrInvalidEnvelope = errors.New("inventory_ingest: invalid envelope")

// ErrMissingExternalID is returned when a record in the batch has no
// external_id. Dedup is impossible without it; we fail the whole call
// rather than silently dropping anonymous records.
var ErrMissingExternalID = errors.New("inventory_ingest: record missing external_id")

// ErrEmptyRecords is returned when the records slice is empty.
var ErrEmptyRecords = errors.New("inventory_ingest: records must not be empty")

// ErrBatchTooLarge is returned when records exceed the per-call cap.
var ErrBatchTooLarge = errors.New("inventory_ingest: batch size exceeds limit")
