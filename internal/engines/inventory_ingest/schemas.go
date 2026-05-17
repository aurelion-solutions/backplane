// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package inventory_ingest

import (
	"fmt"
	"strings"
)

// BatchLimit caps records per single Process call.
const BatchLimit = 50_000

// Request is the input to Service.Process.
//
// CorrelationID may be left empty; the service falls back to the
// caller's context (correlation.Ensure) or generates a fresh UUID.
type Request struct {
	Source        string
	DatasetType   string
	CorrelationID string
	Records       []map[string]any

	// SkipEvent suppresses the post-write
	// `inventory.ingest.batch_received` MQ emit. Set when the caller
	// is going to drive normalization itself in the same request
	// (e.g. the synchronous inventory_import path used by the Lens
	// CSV demo) and does not want the async pipeline running in
	// parallel against the same lake batch.
	SkipEvent bool
}

// Validate checks envelope shape only. Per-record external_id check
// happens later because we need to scan each record anyway.
func (r Request) Validate() error {
	if err := validateIdentifier("source", r.Source, 64); err != nil {
		return err
	}
	if err := validateIdentifier("dataset_type", r.DatasetType, 128); err != nil {
		return err
	}
	if len(r.Records) == 0 {
		return ErrEmptyRecords
	}
	if len(r.Records) > BatchLimit {
		return ErrBatchTooLarge
	}
	return nil
}

// Result is what Process returns. Counts always add up to
// Received = Written + Skipped, and Written = New + Changed.
type Result struct {
	BatchID       string  `json:"batch_id"`
	Source        string  `json:"source"`
	DatasetType   string  `json:"dataset_type"`
	CorrelationID string  `json:"correlation_id"`
	Received      int     `json:"received"`
	Written       int     `json:"written"`
	Skipped       int     `json:"skipped"`
	New           int     `json:"new"`
	Changed       int     `json:"changed"`
	LakeRef       *string `json:"lake_ref,omitempty"`
}

// ListResponse is the GET /ingest/batches envelope.
type ListResponse struct {
	Items  []*IngestBatch `json:"items"`
	Total  int            `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

func validateIdentifier(field, value string, maxLen int) error {
	t := strings.TrimSpace(value)
	if t == "" {
		return fmt.Errorf("%w: %s is required", ErrInvalidEnvelope, field)
	}
	if len(t) > maxLen {
		return fmt.Errorf("%w: %s must be at most %d characters", ErrInvalidEnvelope, field, maxLen)
	}
	return nil
}
