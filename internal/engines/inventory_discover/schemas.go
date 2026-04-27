// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package inventory_discover

import (
	"fmt"
	"strings"
)

// FetchPayload is the POST /discover/runs body.
type FetchPayload struct {
	ConnectorInstanceID string         `json:"connector_instance_id"`
	Operation           string         `json:"operation"`
	DatasetType         string         `json:"dataset_type"`
	Payload             map[string]any `json:"payload,omitempty"`
}

// Validate enforces envelope shape.
func (p FetchPayload) Validate() error {
	if err := validateIdentifier("connector_instance_id", p.ConnectorInstanceID, 255); err != nil {
		return err
	}
	if err := validateIdentifier("operation", p.Operation, 128); err != nil {
		return err
	}
	if err := validateIdentifier("dataset_type", p.DatasetType, 128); err != nil {
		return err
	}
	return nil
}

// ListResponse is the GET /discover/runs envelope.
type ListResponse struct {
	Items  []*DiscoverRun `json:"items"`
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
