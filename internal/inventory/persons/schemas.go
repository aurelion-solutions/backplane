// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package persons

import (
	"fmt"
	"strings"
)

// BulkLimit is the maximum number of items accepted by POST
// /persons/bulk in one request. Matches the kernel cap.
const BulkLimit = 500

// CreatePayload is the POST /persons body.
type CreatePayload struct {
	ExternalID string `json:"external_id"`
	FullName   string `json:"full_name"`
}

// Validate enforces field bounds before the service runs.
func (p CreatePayload) Validate() error {
	if err := validateExternalID(p.ExternalID); err != nil {
		return err
	}
	return validateFullName(p.FullName)
}

// AttributeCreatePayload is the POST /persons/{id}/attributes body.
type AttributeCreatePayload struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Validate enforces field bounds.
func (p AttributeCreatePayload) Validate() error {
	if err := validateAttributeKey(p.Key); err != nil {
		return err
	}
	return validateAttributeValue(p.Value)
}

// BulkItem is one element of POST /persons/bulk.
type BulkItem struct {
	ExternalID string            `json:"external_id"`
	FullName   string            `json:"full_name"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// Validate enforces per-item bounds.
func (b BulkItem) Validate() error {
	if err := validateExternalID(b.ExternalID); err != nil {
		return err
	}
	if err := validateFullName(b.FullName); err != nil {
		return err
	}
	for k, v := range b.Attributes {
		if err := validateAttributeKey(k); err != nil {
			return err
		}
		if err := validateAttributeValue(v); err != nil {
			return err
		}
	}
	return nil
}

// BulkPayload is the POST /persons/bulk body.
type BulkPayload struct {
	Items []BulkItem `json:"items"`
}

// Validate enforces the request envelope: non-empty + ≤ BulkLimit + per-item.
func (p BulkPayload) Validate() error {
	if len(p.Items) == 0 {
		return ErrBulkEmpty
	}
	if len(p.Items) > BulkLimit {
		return ErrBulkTooLarge
	}
	for i, it := range p.Items {
		if err := it.Validate(); err != nil {
			return fmt.Errorf("persons: bulk[%d]: %w", i, err)
		}
	}
	return nil
}

// BulkResult is the response envelope of POST /persons/bulk.
type BulkResult struct {
	RowCount int `json:"row_count"`
}

func validateExternalID(s string) error {
	t := strings.TrimSpace(s)
	if t == "" || len(t) > 255 {
		return fmt.Errorf("persons: external_id must be 1..255 characters")
	}
	return nil
}

func validateFullName(s string) error {
	t := strings.TrimSpace(s)
	if t == "" || len(t) > 255 {
		return fmt.Errorf("persons: full_name must be 1..255 characters")
	}
	return nil
}

func validateAttributeKey(s string) error {
	t := strings.TrimSpace(s)
	if t == "" || len(t) > 255 {
		return fmt.Errorf("persons: attribute key must be 1..255 characters")
	}
	return nil
}

func validateAttributeValue(s string) error {
	if len(s) > 1024 {
		return fmt.Errorf("persons: attribute value must be at most 1024 characters")
	}
	return nil
}
