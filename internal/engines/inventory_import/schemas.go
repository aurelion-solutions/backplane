// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package inventory_import

import "github.com/aurelion-solutions/backplane/internal/engines/inventory_ingest"

// HTTPRequest is the body of POST /api/v0/inventory/import.
type HTTPRequest struct {
	Source        string           `json:"source"`
	DatasetType   string           `json:"dataset_type"`
	CorrelationID string           `json:"correlation_id,omitempty"`
	Records       []map[string]any `json:"records"`
}

// HTTPResponse is the body returned on success.
//
// Ingest carries the verbatim inventory_ingest.Result so the caller
// sees the same counters the async path produces. Normalize is the
// normalize action's own typed Result encoded as map[string]any —
// shape is dataset-specific (employee returns persons_created /
// employments_added / …, account returns its own counts, etc.).
type HTTPResponse struct {
	Ingest    inventory_ingest.Result `json:"ingest"`
	Normalize map[string]any          `json:"normalize"`
}
