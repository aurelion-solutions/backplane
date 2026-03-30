// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package employments

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// BulkLimit caps POST /employments/bulk request size.
const BulkLimit = 500

// CreatePayload is the POST /employments body. Note: no is_locked —
// access locking lives on the Principal, not on the Employment.
type CreatePayload struct {
	PersonID    uuid.UUID  `json:"person_id"`
	Code        string     `json:"code"`
	StartDate   time.Time  `json:"start_date"`
	EndDate     *time.Time `json:"end_date,omitempty"`
	OrgUnitID   *uuid.UUID `json:"org_unit_id,omitempty"`
	Description *string    `json:"description,omitempty"`
}

// Validate enforces field bounds.
func (p CreatePayload) Validate() error {
	if p.PersonID == uuid.Nil {
		return fmt.Errorf("employments: person_id is required")
	}
	if err := validateCode(p.Code); err != nil {
		return err
	}
	if p.StartDate.IsZero() {
		return fmt.Errorf("employments: start_date is required")
	}
	if p.EndDate != nil && p.EndDate.Before(p.StartDate) {
		return ErrInvalidDates
	}
	if p.Description != nil {
		return validateDescription(*p.Description)
	}
	return nil
}

// PatchPayload is the PATCH /employments/{id} body. Aggregate update —
// emits one inventory.employment.updated event with a `changes` map.
type PatchPayload struct {
	Code        *string           `json:"code,omitempty"`
	StartDate   *time.Time        `json:"start_date,omitempty"`
	EndDate     *time.Time        `json:"end_date,omitempty"`
	OrgUnitID   *uuid.UUID        `json:"org_unit_id,omitempty"`
	Description *string           `json:"description,omitempty"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}

// HasAny reports whether at least one mutable field is set.
func (p PatchPayload) HasAny() bool {
	return p.Code != nil || p.StartDate != nil || p.EndDate != nil ||
		p.OrgUnitID != nil || p.Description != nil || p.Attributes != nil
}

// Validate enforces field bounds.
func (p PatchPayload) Validate() error {
	if !p.HasAny() {
		return ErrNoFields
	}
	if p.Code != nil {
		if err := validateCode(*p.Code); err != nil {
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

// AttributeCreatePayload is the POST /employments/{id}/attributes body.
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

// EndPayload is the POST /employments/{id}/end body.
type EndPayload struct {
	EndDate time.Time `json:"end_date"`
}

// BulkItem is one element of POST /employments/bulk. References Person
// (and optional OrgUnit) by external_id so the caller need not know
// the internal UUIDs.
type BulkItem struct {
	PersonExternalID  string            `json:"person_external_id"`
	Code              string            `json:"code"`
	StartDate         time.Time         `json:"start_date"`
	EndDate           *time.Time        `json:"end_date,omitempty"`
	OrgUnitExternalID *string           `json:"org_unit_external_id,omitempty"`
	Description       *string           `json:"description,omitempty"`
	Attributes        map[string]string `json:"attributes,omitempty"`
}

// Validate enforces per-item bounds.
func (b BulkItem) Validate() error {
	if strings.TrimSpace(b.PersonExternalID) == "" {
		return fmt.Errorf("employments: person_external_id is required")
	}
	if err := validateCode(b.Code); err != nil {
		return err
	}
	if b.StartDate.IsZero() {
		return fmt.Errorf("employments: start_date is required")
	}
	if b.EndDate != nil && b.EndDate.Before(b.StartDate) {
		return ErrInvalidDates
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

// BulkPayload is the POST /employments/bulk body.
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
			return fmt.Errorf("employments: bulk[%d]: %w", i, err)
		}
	}
	return nil
}

// BulkResult is the POST /employments/bulk response envelope.
type BulkResult struct {
	RowCount int `json:"row_count"`
}

func validateCode(s string) error {
	t := strings.TrimSpace(s)
	if t == "" || len(t) > 64 {
		return ErrCodeRequired
	}
	return nil
}

func validateDescription(s string) error {
	if len(s) > 255 {
		return fmt.Errorf("employments: description must be at most 255 characters")
	}
	return nil
}

func validateAttributeKey(s string) error {
	t := strings.TrimSpace(s)
	if t == "" || len(t) > 255 {
		return fmt.Errorf("employments: attribute key must be 1..255 characters")
	}
	return nil
}

func validateAttributeValue(s string) error {
	if len(s) > 1024 {
		return fmt.Errorf("employments: attribute value must be at most 1024 characters")
	}
	return nil
}
