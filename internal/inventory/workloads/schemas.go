// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package workloads

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// BulkLimit caps POST /workloads/bulk request size.
const BulkLimit = 500

// CreatePayload is the POST /workloads body.
type CreatePayload struct {
	ExternalID        string     `json:"external_id"`
	Name              string     `json:"name"`
	Description       *string    `json:"description,omitempty"`
	OwnerEmploymentID *uuid.UUID `json:"owner_employment_id,omitempty"`
	ApplicationID     *uuid.UUID `json:"application_id,omitempty"`
}

// Validate enforces field bounds.
func (p CreatePayload) Validate() error {
	if err := validateExternalID(p.ExternalID); err != nil {
		return err
	}
	if err := validateName(p.Name); err != nil {
		return err
	}
	if p.Description != nil {
		return validateDescription(*p.Description)
	}
	return nil
}

// PatchPayload is the PATCH /workloads/{id} body.
type PatchPayload struct {
	Name              *string           `json:"name,omitempty"`
	Description       *string           `json:"description,omitempty"`
	OwnerEmploymentID *uuid.UUID        `json:"owner_employment_id,omitempty"`
	ApplicationID     *uuid.UUID        `json:"application_id,omitempty"`
	Attributes        map[string]string `json:"attributes,omitempty"`
}

// HasAny reports whether at least one mutable field is present.
func (p PatchPayload) HasAny() bool {
	return p.Name != nil || p.Description != nil ||
		p.OwnerEmploymentID != nil || p.ApplicationID != nil || p.Attributes != nil
}

// Validate enforces field bounds on the present fields only.
func (p PatchPayload) Validate() error {
	if !p.HasAny() {
		return ErrNoFields
	}
	if p.Name != nil {
		if err := validateName(*p.Name); err != nil {
			return err
		}
	}
	if p.Description != nil {
		if err := validateDescription(*p.Description); err != nil {
			return err
		}
	}
	for k, v := range p.Attributes {
		if err := validateAttributeKey(k); err != nil {
			return err
		}
		if err := validateAttributeValue(v); err != nil {
			return err
		}
	}
	return nil
}

// AttributeCreatePayload is the POST /workloads/{id}/attributes body.
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

// BulkItem is one element of POST /workloads/bulk.
type BulkItem struct {
	ExternalID        string            `json:"external_id"`
	Name              string            `json:"name"`
	Description       *string           `json:"description,omitempty"`
	OwnerEmploymentID *uuid.UUID        `json:"owner_employment_id,omitempty"`
	ApplicationID     *uuid.UUID        `json:"application_id,omitempty"`
	Attributes        map[string]string `json:"attributes,omitempty"`
}

// Validate enforces per-item bounds.
func (b BulkItem) Validate() error {
	if err := validateExternalID(b.ExternalID); err != nil {
		return err
	}
	if err := validateName(b.Name); err != nil {
		return err
	}
	if b.Description != nil {
		if err := validateDescription(*b.Description); err != nil {
			return err
		}
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

// BulkPayload is the POST /workloads/bulk body.
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
			return fmt.Errorf("workloads: bulk[%d]: %w", i, err)
		}
	}
	return nil
}

// BulkResult is the POST /workloads/bulk response envelope.
type BulkResult struct {
	RowCount int `json:"row_count"`
}

func validateExternalID(s string) error {
	t := strings.TrimSpace(s)
	if t == "" || len(t) > 255 {
		return fmt.Errorf("workloads: external_id must be 1..255 characters")
	}
	return nil
}

func validateName(s string) error {
	t := strings.TrimSpace(s)
	if t == "" || len(t) > 255 {
		return fmt.Errorf("workloads: name must be 1..255 characters")
	}
	return nil
}

func validateDescription(s string) error {
	if len(s) > 255 {
		return fmt.Errorf("workloads: description must be at most 255 characters")
	}
	return nil
}

func validateAttributeKey(s string) error {
	t := strings.TrimSpace(s)
	if t == "" || len(t) > 255 {
		return fmt.Errorf("workloads: attribute key must be 1..255 characters")
	}
	return nil
}

func validateAttributeValue(s string) error {
	if len(s) > 1024 {
		return fmt.Errorf("workloads: attribute value must be at most 1024 characters")
	}
	return nil
}
