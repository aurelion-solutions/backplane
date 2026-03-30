// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package employee_records

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// BulkLimit caps POST /employee-records/bulk request size.
const BulkLimit = 500

// CreatePayload is the POST /employee-records body.
type CreatePayload struct {
	ExternalID    string    `json:"external_id"`
	ApplicationID uuid.UUID `json:"application_id"`
	Description   *string   `json:"description,omitempty"`
}

// Validate enforces field bounds.
func (p CreatePayload) Validate() error {
	if err := validateExternalID(p.ExternalID); err != nil {
		return err
	}
	if p.ApplicationID == uuid.Nil {
		return fmt.Errorf("employee_records: application_id is required")
	}
	if p.Description != nil {
		return validateDescription(*p.Description)
	}
	return nil
}

// AttributeCreatePayload is the POST attribute body.
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

// MappingCreatePayload is the POST
// /applications/{id}/employee-record-mappings body.
type MappingCreatePayload struct {
	EmployeeRecordKey string `json:"employee_record_key"`
	PersonKey         string `json:"person_key"`
	IsDeterminator    bool   `json:"is_determinator"`
	AllowUpstream     bool   `json:"allow_upstream"`
}

// Validate enforces field bounds.
func (p MappingCreatePayload) Validate() error {
	if err := validateAttributeKey(p.EmployeeRecordKey); err != nil {
		return fmt.Errorf("employee_records: employee_record_key: %w", err)
	}
	return validateAttributeKey(p.PersonKey)
}

// MatchCreatePayload is the POST /employee-records/{id}/match body for
// a manual match (resolver-bypassing). person_id is the canonical
// human; employment_id is the specific mask the record represents.
type MatchCreatePayload struct {
	PersonID               uuid.UUID `json:"person_id"`
	EmploymentID           uuid.UUID `json:"employment_id"`
	MatchedViaDeterminator bool      `json:"matched_via_determinator"`
}

// Validate enforces required fields.
func (p MatchCreatePayload) Validate() error {
	if p.PersonID == uuid.Nil {
		return fmt.Errorf("employee_records: person_id is required")
	}
	if p.EmploymentID == uuid.Nil {
		return fmt.Errorf("employee_records: employment_id is required")
	}
	return nil
}

// BulkItem is one element of POST /employee-records/bulk. Application
// is referenced by external_id (`application_code` matches kernel's
// Application.code unique key).
type BulkItem struct {
	ApplicationCode string            `json:"application_code"`
	ExternalID      string            `json:"external_id"`
	Description     *string           `json:"description,omitempty"`
	Attributes      map[string]string `json:"attributes,omitempty"`
}

// Validate enforces per-item bounds.
func (b BulkItem) Validate() error {
	if strings.TrimSpace(b.ApplicationCode) == "" {
		return fmt.Errorf("employee_records: application_code is required")
	}
	if err := validateExternalID(b.ExternalID); err != nil {
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

// BulkPayload is the POST /employee-records/bulk body.
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
			return fmt.Errorf("employee_records: bulk[%d]: %w", i, err)
		}
	}
	return nil
}

// BulkResult is the POST /employee-records/bulk response envelope.
type BulkResult struct {
	RowCount int `json:"row_count"`
}

// ResolveResult is the response envelope from POST
// /employee-records/{id}/resolve.
type ResolveResult struct {
	EmployeeRecordID       uuid.UUID  `json:"employee_record_id"`
	PersonID               *uuid.UUID `json:"person_id,omitempty"`
	EmploymentID           *uuid.UUID `json:"employment_id,omitempty"`
	MatchedViaDeterminator bool       `json:"matched_via_determinator"`
	Resolved               bool       `json:"resolved"`
}

func validateExternalID(s string) error {
	t := strings.TrimSpace(s)
	if t == "" || len(t) > 255 {
		return fmt.Errorf("employee_records: external_id must be 1..255 characters")
	}
	return nil
}

func validateDescription(s string) error {
	if len(s) > 255 {
		return fmt.Errorf("employee_records: description must be at most 255 characters")
	}
	return nil
}

func validateAttributeKey(s string) error {
	t := strings.TrimSpace(s)
	if t == "" || len(t) > 255 {
		return fmt.Errorf("attribute key must be 1..255 characters")
	}
	return nil
}

func validateAttributeValue(s string) error {
	if len(s) > 1024 {
		return fmt.Errorf("employee_records: attribute value must be at most 1024 characters")
	}
	return nil
}
