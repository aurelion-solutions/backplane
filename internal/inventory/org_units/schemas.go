// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package org_units

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// BulkLimit is the maximum number of items per POST /org-units/bulk.
const BulkLimit = 500

// CreatePayload is the POST /org-units body. Only external nodes are
// writable via the API; the service stamps is_internal=false.
type CreatePayload struct {
	ExternalID  string     `json:"external_id"`
	Name        string     `json:"name"`
	ParentID    *uuid.UUID `json:"parent_id,omitempty"`
	Description *string    `json:"description,omitempty"`
}

// Validate enforces field bounds.
func (p CreatePayload) Validate() error {
	if err := validateExternalID(p.ExternalID); err != nil {
		return err
	}
	return validateName(p.Name)
}

// PatchPayload is the PATCH /org-units/{id} body. external_id,
// is_internal and parent_id are read-only after creation.
type PatchPayload struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// HasAny reports whether at least one field is set.
func (p PatchPayload) HasAny() bool { return p.Name != nil || p.Description != nil }

// Validate enforces field bounds on the present fields only.
func (p PatchPayload) Validate() error {
	if !p.HasAny() {
		return ErrNoFields
	}
	if p.Name != nil {
		return validateName(*p.Name)
	}
	return nil
}

// BulkItem is one element of POST /org-units/bulk. Parent is referenced
// by external_id so the entire tree can be supplied in one call without
// requiring the caller to know the database UUIDs.
type BulkItem struct {
	ExternalID       string  `json:"external_id"`
	Name             string  `json:"name"`
	ParentExternalID *string `json:"parent_external_id,omitempty"`
	Description      *string `json:"description,omitempty"`
}

// Validate enforces per-item bounds, including the self-reference rule.
func (b BulkItem) Validate() error {
	if err := validateExternalID(b.ExternalID); err != nil {
		return err
	}
	if err := validateName(b.Name); err != nil {
		return err
	}
	if b.ParentExternalID != nil && *b.ParentExternalID == b.ExternalID {
		return ErrSelfReference
	}
	return nil
}

// BulkPayload is the POST /org-units/bulk body.
type BulkPayload struct {
	Items []BulkItem `json:"items"`
}

// Validate enforces envelope-level bounds.
func (p BulkPayload) Validate() error {
	if len(p.Items) == 0 {
		return ErrBulkEmpty
	}
	if len(p.Items) > BulkLimit {
		return ErrBulkTooLarge
	}
	for i, it := range p.Items {
		if err := it.Validate(); err != nil {
			return fmt.Errorf("org_units: bulk[%d]: %w", i, err)
		}
	}
	return nil
}

// BulkResult is the POST /org-units/bulk response envelope.
type BulkResult struct {
	RowCount int `json:"row_count"`
}

func validateExternalID(s string) error {
	t := strings.TrimSpace(s)
	if t == "" || len(t) > 255 {
		return fmt.Errorf("org_units: external_id must be 1..255 characters")
	}
	return nil
}

func validateName(s string) error {
	t := strings.TrimSpace(s)
	if t == "" || len(t) > 255 {
		return fmt.Errorf("org_units: name must be 1..255 characters")
	}
	return nil
}
