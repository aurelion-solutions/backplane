// SPDX-FileCopyrightText: 2026 Michael Abramovich
//
// SPDX-License-Identifier: BUSL-1.1

package customers

import (
	"fmt"
	"strings"

	"github.com/aurelion-solutions/backplane/internal/inventory/shared"
)

// BulkLimit caps POST /customers/bulk request size.
const BulkLimit = 500

// CreatePayload is the POST /customers body. No is_locked — access
// blocking lives on the Principal, not on the Customer body.
type CreatePayload struct {
	ExternalID    string                     `json:"external_id"`
	EmailVerified *bool                      `json:"email_verified,omitempty"`
	TenantID      *string                    `json:"tenant_id,omitempty"`
	TenantRole    *shared.CustomerTenantRole `json:"tenant_role,omitempty"`
	PlanTier      *shared.CustomerPlanTier   `json:"plan_tier,omitempty"`
	MFAEnabled    *bool                      `json:"mfa_enabled,omitempty"`
	Description   *string                    `json:"description,omitempty"`
}

// Validate enforces field bounds.
func (p CreatePayload) Validate() error {
	if err := validateExternalID(p.ExternalID); err != nil {
		return err
	}
	if p.Description != nil {
		if err := validateDescription(*p.Description); err != nil {
			return err
		}
	}
	if p.TenantRole != nil && !p.TenantRole.Valid() {
		return fmt.Errorf("customers: %w: tenant_role=%q", ErrInvalidEnum, *p.TenantRole)
	}
	if p.PlanTier != nil && !p.PlanTier.Valid() {
		return fmt.Errorf("customers: %w: plan_tier=%q", ErrInvalidEnum, *p.PlanTier)
	}
	return nil
}

// PatchPayload is the PATCH /customers/{id} body. Strict 3-field
// vocabulary: tenant_id / tenant_role / external_id / created_at are
// not patchable; is_locked moved to the Principal layer.
type PatchPayload struct {
	EmailVerified *bool                    `json:"email_verified,omitempty"`
	MFAEnabled    *bool                    `json:"mfa_enabled,omitempty"`
	PlanTier      *shared.CustomerPlanTier `json:"plan_tier,omitempty"`
}

// HasAny reports whether at least one mutable field is set.
func (p PatchPayload) HasAny() bool {
	return p.EmailVerified != nil || p.MFAEnabled != nil || p.PlanTier != nil
}

// Validate enforces field bounds on the present fields only.
func (p PatchPayload) Validate() error {
	if !p.HasAny() {
		return ErrNoFields
	}
	if p.PlanTier != nil && !p.PlanTier.Valid() {
		return fmt.Errorf("customers: %w: plan_tier=%q", ErrInvalidEnum, *p.PlanTier)
	}
	return nil
}

// AttributeCreatePayload is the POST /customers/{id}/attributes body.
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

// BulkItem is one element of POST /customers/bulk.
type BulkItem struct {
	ExternalID    string                     `json:"external_id"`
	EmailVerified *bool                      `json:"email_verified,omitempty"`
	TenantID      *string                    `json:"tenant_id,omitempty"`
	TenantRole    *shared.CustomerTenantRole `json:"tenant_role,omitempty"`
	PlanTier      *shared.CustomerPlanTier   `json:"plan_tier,omitempty"`
	MFAEnabled    *bool                      `json:"mfa_enabled,omitempty"`
	Description   *string                    `json:"description,omitempty"`
	Attributes    map[string]string          `json:"attributes,omitempty"`
}

// Validate enforces per-item bounds.
func (b BulkItem) Validate() error {
	if err := validateExternalID(b.ExternalID); err != nil {
		return err
	}
	if b.TenantRole != nil && !b.TenantRole.Valid() {
		return fmt.Errorf("customers: %w: tenant_role=%q", ErrInvalidEnum, *b.TenantRole)
	}
	if b.PlanTier != nil && !b.PlanTier.Valid() {
		return fmt.Errorf("customers: %w: plan_tier=%q", ErrInvalidEnum, *b.PlanTier)
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

// BulkPayload is the POST /customers/bulk body.
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
			return fmt.Errorf("customers: bulk[%d]: %w", i, err)
		}
	}
	return nil
}

// BulkResult is the POST /customers/bulk response envelope.
type BulkResult struct {
	RowCount int `json:"row_count"`
}

func validateExternalID(s string) error {
	t := strings.TrimSpace(s)
	if t == "" || len(t) > 255 {
		return fmt.Errorf("customers: external_id must be 1..255 characters")
	}
	return nil
}

func validateDescription(s string) error {
	if len(s) > 255 {
		return fmt.Errorf("customers: description must be at most 255 characters")
	}
	return nil
}

func validateAttributeKey(s string) error {
	t := strings.TrimSpace(s)
	if t == "" || len(t) > 255 {
		return fmt.Errorf("customers: attribute key must be 1..255 characters")
	}
	return nil
}

func validateAttributeValue(s string) error {
	if len(s) > 1024 {
		return fmt.Errorf("customers: attribute value must be at most 1024 characters")
	}
	return nil
}
